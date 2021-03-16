package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/security"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	istiosecurity "istio.io/istio/pkg/security"
	"istio.io/istio/security/pkg/nodeagent/plugin/providers/google/stsclient"
	"istio.io/istio/security/pkg/stsservice"
	"istio.io/istio/security/pkg/stsservice/server"
	"istio.io/istio/security/pkg/stsservice/tokenmanager/google"
	"k8s.io/api/admission/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	pb "istio.io/api/security/v1alpha1"
	"istio.io/pkg/log"
)

type PodSpec struct {
	ServiceAccount string
	Node           string
	App            string
	Namespace      string
	UID            string
	IP             string
	PodType        model.PodType
}

type Pod struct {
	Spec *PodSpec
	// For internal optimization around closing only
	created bool
	xds     *xds.Simulation
}

var _ model.Simulation = &Pod{}

func NewPod(s PodSpec) *Pod {
	if s.UID == "" {
		s.UID = util.GenUID()
	}
	if s.IP == "" {
		s.IP = util.GetIP()
	}
	return &Pod{
		Spec: &s,
	}
}

type GrpcCredentials struct {
	Metadata func() (map[string]string, error)
}

func (g GrpcCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return g.Metadata()
}

func (g GrpcCredentials) RequireTransportSecurity() bool {
	return true
}

var _ credentials.PerRPCCredentials = &GrpcCredentials{}

func (p *Pod) Run(ctx model.Context) (err error) {
	pod := p.getPod()

	if err = ctx.Client.ApplyFast(pod); err != nil {
		return fmt.Errorf("failed to apply config: %v", err)
	}
	projectNumber := "700341650140"
	trustDomain := "howardjohn-istio.svc.id.goog"
	clusterURL := "https://container.googleapis.com/v1/projects/howardjohn-istio/locations/us-central1-c/clusters/howardjohn"
	tmp, err := google.CreateTokenManagerPlugin(nil, trustDomain,
		projectNumber, clusterURL, true)
	if err != nil {
		return err
	}
	p.created = true

	if p.Spec.PodType != model.ExternalType {
		if err := sendInjectionRequest(ctx.Args.InjectAddress, pod); err != nil {
			return err
		}

		p.xds = &xds.Simulation{
			Labels:    pod.Labels,
			Namespace: pod.Namespace,
			Name:      pod.Name,
			IP:        p.Spec.IP,
			PodType:   p.Spec.PodType,
			// TODO: multicluster
			Cluster: "Kubernetes",
			Metadata: map[string]interface{}{
				"CLOUDRUN_ADDR": "asm-howardjohn-asm-managed-us-central1-c-p7morfnxsq-uc.a.run.app:443",
			},
		}

		_, port, _ := net.SplitHostPort(ctx.Args.PilotAddress)
		if port == "443" {
			token, err := security.GetServiceAccountToken(ctx.Client, pod.Namespace, p.Spec.ServiceAccount)
			if err != nil {
				return err
			}
			params := istiosecurity.StsRequestParameters{
				Scope:            stsclient.Scope,
				GrantType:        server.TokenExchangeGrantType,
				SubjectToken:     strings.TrimSpace(token),
				SubjectTokenType: server.SubjectTokenType,
			}
			et, err := tmp.ExchangeToken(params)
			if err != nil {
				return err
			}
			respData := &stsservice.StsResponseParameters{}
			if err := json.Unmarshal(et, respData); err != nil {
				return fmt.Errorf("failed to unmarshal access token response data: %v", err)
			}
			p.xds.GrpcOpts = []grpc.DialOption{grpc.WithPerRPCCredentials(GrpcCredentials{Metadata: func() (map[string]string, error) {
				// TODO generate each time
				meta, err := tmp.GetMetadata(false, google.GCPAuthProvider, respData.AccessToken)
				log.Infof("%+v", meta)
				return meta, err
			}})}
		} else if port != "15010" {
			t0 := time.Now()
			cert, rootCert, err := sendCsr(ctx, p.Spec)
			if err != nil {
				return err
			}
			log.Debugf("csr for %v complete in %v", pod.Name, time.Since(t0))
			p.xds.ClientCert = cert
			p.xds.RootCert = rootCert
		}
		return p.xds.Run(ctx)
	} else {
		log.Infof("Starting pod %v", pod.Name)
	}
	return nil
}

func sendCsr(ctx model.Context, s *PodSpec) (tls.Certificate, []byte, error) {
	rootCert, err := security.GetRootCert(ctx.Client)
	if err != nil {
		return tls.Certificate{}, nil, errors.Wrap(err, "failed to fetch root cert")
	}

	token, err := security.GetServiceAccountToken(ctx.Client, s.Namespace, s.ServiceAccount)
	if err != nil {
		return tls.Certificate{}, nil, errors.Wrap(err, "failed to create service account token")
	}

	kp, err := security.GenerateKey(s.Namespace, s.ServiceAccount)
	if err != nil {
		return tls.Certificate{}, nil, errors.Wrap(err, "failed to create csr")
	}
	client, err := newCitadelClient(ctx.Args.PilotAddress, []byte(rootCert))
	if err != nil {
		return tls.Certificate{}, nil, errors.Wrap(err, "creating citadel client")
	}
	req := &pb.IstioCertificateRequest{
		Csr:              string(kp.CsrPEM),
		ValidityDuration: int64((time.Hour * 24 * 7).Seconds()),
	}
	rctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("Authorization", "Bearer "+token, "ClusterID", "Kubernetes"))
	resp, err := client.CreateCertificate(rctx, req)
	if err != nil {
		return tls.Certificate{}, nil, errors.Wrap(err, "send CSR")
	}
	certChain := []byte{}
	for _, c := range resp.CertChain {
		certChain = append(certChain, []byte(c)...)
	}

	clientCert, err := tls.X509KeyPair(certChain, kp.KeyPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	return clientCert, []byte(rootCert), nil
}

// NewCitadelClient create a CA client for Citadel.
func newCitadelClient(endpoint string, rootCert []byte) (pb.IstioCertificateServiceClient, error) {
	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(rootCert)
	if !ok {
		return nil, fmt.Errorf("failed to append certificates")
	}
	config := tls.Config{
		RootCAs:            certPool,
		InsecureSkipVerify: true,
	}
	transportCreds := credentials.NewTLS(&config)

	// TODO(JimmyCYJ): This connection is create at construction time. If conn is broken at anytime,
	//  need a way to reconnect.
	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(transportCreds))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to endpoint %s", endpoint)
	}

	client := pb.NewIstioCertificateServiceClient(conn)
	return client, nil
}

func (p *Pod) Cleanup(ctx model.Context) error {
	if p.created {
		if err := ctx.Client.Delete(p.getPod()); err != nil {
			return err
		}
	}
	if p.Spec.PodType != model.ExternalType {
		return p.xds.Cleanup(ctx)
	}
	return nil
}

func (p *Pod) Name() string {
	return fmt.Sprintf("%s-%s", p.Spec.App, p.Spec.UID)
}

func (p *Pod) getPod() *v1.Pod {
	s := p.Spec
	labels := map[string]string{
		"app": s.App,
	}
	if p.Spec.PodType == model.SidecarType {
		labels["security.istio.io/tlsMode"] = "istio"
	}
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.Name(),
			Namespace: s.Namespace,
			Labels:    labels,
		},
		Spec: v1.PodSpec{
			ServiceAccountName: s.ServiceAccount,
			InitContainers: []v1.Container{{
				Name:  "istio-init",
				Image: "istio/proxyv2",
			}},
			Containers: []v1.Container{{
				Name:  "app",
				Image: "app",
			}, {
				Name:  "istio-proxy",
				Image: "istio/proxyv2",
			}},
			NodeName: s.Node,
		},
		Status: v1.PodStatus{
			Phase:      v1.PodRunning,
			Conditions: nil,
			PodIP:      s.IP,
			PodIPs:     []v1.PodIP{{s.IP}},
		},
	}
}

var client = http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

func sendInjectionRequest(address string, pod *v1.Pod) error {
	if address == "" {
		return nil
	}
	jbytes, err := json.Marshal(pod)
	if err != nil {
		return err
	}
	request := &v1beta1.AdmissionRequest{
		UID:                types.UID(util.GenUID()),
		Kind:               metav1.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Resource:           metav1.GroupVersionResource{Version: "v1", Resource: "pods"},
		RequestKind:        &metav1.GroupVersionKind{Version: "v1", Kind: "Pod"},
		RequestResource:    &metav1.GroupVersionResource{Version: "v1", Resource: "pods"},
		RequestSubResource: "",
		Name:               pod.Name,
		Namespace:          pod.Namespace,
		Operation:          v1beta1.Create,
		Object:             runtime.RawExtension{Raw: jbytes},
		DryRun:             util.BoolPointer(false),
		Options:            runtime.RawExtension{},
	}
	requestBytes, err := json.Marshal(&v1beta1.AdmissionReview{Request: request})
	if err != nil {
		return err
	}
	log.Infof("%v", string(requestBytes))
	req, err := http.NewRequest(http.MethodPost, address, bytes.NewReader(requestBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("got bad response to injection: %v", resp.StatusCode)
	}
	return nil
}
