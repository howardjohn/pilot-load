package app

import (
	"reflect"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type EndpointSpec struct {
	App       string
	Namespace string
	// Map of pod name to IP
	Infos         map[string]podInfo
	ClusterType model.ClusterType
}

type Endpoint struct {
	Spec *EndpointSpec
}

var _ model.Simulation = &Endpoint{}

func NewEndpoint(s EndpointSpec) *Endpoint {
	return &Endpoint{Spec: &s}
}

func (e *Endpoint) SetAddresses(ctx model.Context, infos map[string]podInfo) error {
	if reflect.DeepEqual(e.Spec.Infos, infos) {
		return nil
	}
	e.Spec.Infos = infos
	return e.Run(ctx)
}

func (e *Endpoint) Run(ctx model.Context) (err error) {
	return kube.Apply(ctx.Client, e.getEndpoint())
}

func (e *Endpoint) Cleanup(ctx model.Context) error {
	return kube.Delete(ctx.Client, e.getEndpoint())
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
	for pod, i := range s.Infos {
		addr := v1.EndpointAddress{
			IP:       i.ip,
			NodeName: &i.node,
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
	if len(s.Infos) > 0 {
		ep.Subsets = append(ep.Subsets, subset)
	}
	return ep
}
