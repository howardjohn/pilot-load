package app

import (
	"reflect"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type EndpointSpec struct {
	Node      string
	App       string
	Namespace string
	// Map of pod name to IP
	IPs         map[string]string
	ClusterType model.ClusterType
}

type Endpoint struct {
	Spec *EndpointSpec
}

var _ model.Simulation = &Endpoint{}

func NewEndpoint(s EndpointSpec) *Endpoint {
	return &Endpoint{Spec: &s}
}

func (e *Endpoint) SetAddresses(ctx model.Context, ips map[string]string) error {
	if reflect.DeepEqual(e.Spec.IPs, ips) {
		return nil
	}
	e.Spec.IPs = ips
	return e.Run(ctx)
}

func (e *Endpoint) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(e.getEndpoint())
}

func (e *Endpoint) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(e.getEndpoint())
}

func (e *Endpoint) getEndpoint() *v1.Endpoints {
	s := e.Spec
	ep := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.App,
			Namespace: s.Namespace,
			Labels: map[string]string{
				"app":                       s.App,
				"security.istio.io/tlsMode": "istio",
			},
		},
	}
	subset := v1.EndpointSubset{}
	for pod, ip := range s.IPs {
		addr := v1.EndpointAddress{
			IP:       ip,
			NodeName: &s.Node,
		}
		if e.Spec.ClusterType != model.Real {
			// We will make a selector-less service+endpoint if in a real cluster
			addr.TargetRef = &v1.ObjectReference{
				Kind:      "Pod",
				Namespace: s.Namespace,
				Name:      pod,
			}
		}
		subset.Addresses = append(subset.Addresses, addr)
	}
	subset.Ports = []v1.EndpointPort{{
		Name: "http",
		Port: 80,
	}, {
		Name: "https",
		Port: 443,
	}}
	if len(s.IPs) > 0 {
		ep.Subsets = append(ep.Subsets, subset)
	}
	return ep
}
