package app

import (
	"reflect"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type EndpointSpec struct {
	Node      string
	App       string
	Namespace string
	IPs       []string
}

type Endpoint struct {
	Spec    *EndpointSpec
	running bool
}

var _ model.Simulation = &Endpoint{}

func NewEndpoint(s EndpointSpec) *Endpoint {
	return &Endpoint{Spec: &s}
}

func (e *Endpoint) SetAddresses(ctx model.Context, ips []string) error {
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
		},
	}
	subset := v1.EndpointSubset{}
	for _, ip := range s.IPs {
		subset.Addresses = append(subset.Addresses, v1.EndpointAddress{IP: ip, NodeName: &s.Node})
	}
	subset.Ports = []v1.EndpointPort{{
		Name: "http",
		Port: 80,
	}}
	if len(s.IPs) > 0 {
		ep.Subsets = append(ep.Subsets, subset)
	}
	return ep
}
