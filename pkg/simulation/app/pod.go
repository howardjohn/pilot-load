package app

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"

	"istio.io/pkg/log"
	"k8s.io/api/admission/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"
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

func (p *Pod) Run(ctx model.Context) (err error) {
	pod := p.getPod()

	if err = ctx.Client.Apply(pod); err != nil {
		return fmt.Errorf("failed to apply config: %v", err)
	}

	p.created = true

	if p.Spec.PodType != model.ExternalType {
		if err := sendInjectionRequest(ctx.Args.InjectAddress, p.getPod()); err != nil {
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
		}
		return p.xds.Run(ctx)
	}
	return nil
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
			Volumes: nil,
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
