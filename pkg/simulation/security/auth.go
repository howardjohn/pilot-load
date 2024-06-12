package security

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	pb "istio.io/api/security/v1alpha1"
	"istio.io/istio/pkg/security"
	"istio.io/istio/security/pkg/nodeagent/plugin/providers/google/stsclient"
	pkiutil "istio.io/istio/security/pkg/pki/util"
	"istio.io/istio/security/pkg/stsservice"
	"istio.io/istio/security/pkg/stsservice/server"
	"istio.io/istio/security/pkg/stsservice/tokenmanager/google"

	"github.com/howardjohn/pilot-load/pkg/kube"
)

type AuthOptions struct {
	Type   AuthType
	Client *kube.Client
}

type AuthType string

const (
	AuthTypeDefault      AuthType = ""
	AuthTypePlaintext    AuthType = "plaintext"
	AuthTypeMTLS         AuthType = "mtls"
	AuthTypeJWT          AuthType = "jwt"
	AuthTypePlaintextJWT AuthType = "plaintext-jwt"
)

func DefaultAuthForAddress(addr string) AuthType {
	_, port, _ := net.SplitHostPort(addr)
	switch port {
	case "15010":
		return AuthTypePlaintext
	default:
		return AuthTypeJWT
	}
}

func AuthTypeOptions() []AuthType {
	return []AuthType{AuthTypePlaintext, AuthTypeMTLS, AuthTypeJWT, AuthTypePlaintextJWT}
}

func (a *AuthOptions) Certificate(fetchRoot func() (string, error), addr, serviceAccount, namespace string) (Cert, error) {
	rootCert, err := fetchRoot()
	if err != nil {
		return Cert{}, fmt.Errorf("failed to fetch root cert: %v", err)
	}

	token, err := GetServiceAccountToken(a.Client, "istio-ca", namespace, serviceAccount)
	if err != nil {
		return Cert{}, err
	}

	san := fmt.Sprintf("spiffe://%s/ns/%s/sa/%s", "cluster.local", namespace, serviceAccount)
	options := pkiutil.CertOptions{
		Host:       san,
		RSAKeySize: 2048,
	}
	// Generate the cert/key, send CSR to CA.
	csrPEM, keyPEM, err := pkiutil.GenCSR(options)
	if err != nil {
		return Cert{}, err
	}
	client, err := newCitadelClient(addr, []byte(rootCert))
	if err != nil {
		return Cert{}, fmt.Errorf("creating citadel client: %v", err)
	}
	req := &pb.IstioCertificateRequest{
		Csr:              string(csrPEM),
		ValidityDuration: int64((time.Hour * 24 * 7).Seconds()),
	}
	rctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("Authorization", "Bearer "+token, "ClusterID", "Kubernetes"))
	resp, err := client.CreateCertificate(rctx, req)
	if err != nil {
		return Cert{}, fmt.Errorf("send CSR: %v", err)
	}
	certChain := []byte{}
	for _, c := range resp.CertChain {
		certChain = append(certChain, []byte(c)...)
	}
	return Cert{certChain, keyPEM, []byte(rootCert)}, nil
}

type Cert struct {
	ClientCert, Key, RootCert []byte
}

func (a *AuthOptions) GrpcOptions(serviceAccount, namespace string) []grpc.DialOption {
	insecureTls := grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	}))
	switch a.Type {
	case AuthTypePlaintext:
		return []grpc.DialOption{grpc.WithInsecure()}
	case AuthTypeMTLS:
		panic(AuthTypeMTLS + " is not currently implemented")
	case AuthTypeGoogle:
		fetch := func() (map[string]string, error) {
			t, err := GetServiceAccountToken(a.Client, a.TrustDomain, namespace, serviceAccount)
			if err != nil {
				return nil, err
			}
			params := security.StsRequestParameters{
				Scope:            stsclient.Scope,
				GrantType:        server.TokenExchangeGrantType,
				SubjectToken:     strings.TrimSpace(t),
				SubjectTokenType: server.SubjectTokenType,
			}
			et, err := a.tokenManager.ExchangeToken(params)
			if err != nil {
				return nil, err
			}
			respData := &stsservice.StsResponseParameters{}
			if err := json.Unmarshal(et, respData); err != nil {
				return nil, fmt.Errorf("failed to unmarshal access token response data: %v", err)
			}
			meta, err := a.tokenManager.GetMetadata(false, google.GCPAuthProvider, respData.AccessToken)
			return meta, err
		}
		return []grpc.DialOption{insecureTls, grpc.WithPerRPCCredentials(grpcCredentials{fetch})}
	case AuthTypeJWT:
		fetch := func() (map[string]string, error) {
			token, err := GetServiceAccountToken(a.Client, "istio-ca", namespace, serviceAccount)
			if err != nil {
				return nil, err
			}
			return map[string]string{
				"authorization": "Bearer " + token,
			}, nil
		}
		return []grpc.DialOption{insecureTls, grpc.WithPerRPCCredentials(grpcCredentials{fetch})}
	case AuthTypePlaintextJWT:
		fetch := func() (map[string]string, error) {
			token, err := GetServiceAccountToken(a.Client, "istio-ca", namespace, serviceAccount)
			if err != nil {
				return nil, err
			}
			return map[string]string{
				"authorization": "Bearer " + token,
			}, nil
		}
		return []grpc.DialOption{grpc.WithInsecure(), grpc.WithPerRPCCredentials(grpcCredentials{fetch})}
	default:
		panic("unknown auth type: " + a.Type)
	}
}

type grpcCredentials struct {
	Metadata func() (map[string]string, error)
}

func (g grpcCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return g.Metadata()
}

func (g grpcCredentials) RequireTransportSecurity() bool {
	return false
}
