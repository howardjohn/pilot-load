package config

import (
	"strings"

	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type HTTPRouteSpec struct {
	App       string
	Namespace string
	Gateways  []string
	Weight    int
}

type HTTPRoute struct {
	Spec *HTTPRouteSpec
}

var (
	_ model.Simulation            = &HTTPRoute{}
	_ model.RefreshableSimulation = &HTTPRoute{}
)

func NewHTTPRoute(s HTTPRouteSpec) *HTTPRoute {
	if s.Weight == 0 {
		s.Weight = 1
	}
	return &HTTPRoute{Spec: &s}
}

func (v *HTTPRoute) Run(ctx model.Context) (err error) {
	return kube.Apply(ctx.Client, v.getHTTPRoute())
}

func (v *HTTPRoute) Cleanup(ctx model.Context) error {
	return kube.Delete(ctx.Client, v.getHTTPRoute())
}

func (v *HTTPRoute) Refresh(ctx model.Context) error {
	v.Spec.Weight += 1
	return v.Run(ctx)
}

func (v *HTTPRoute) getHTTPRoute() *gateway.HTTPRoute {
	s := v.Spec
	routes := []gateway.HTTPRouteRule{{
		BackendRefs: []gateway.HTTPBackendRef{{
			BackendRef: gateway.BackendRef{
				BackendObjectReference: gateway.BackendObjectReference{
					Name: gateway.ObjectName(s.App),
					Port: ptr.Of(gateway.PortNumber(80)),
				},
				Weight:  ptr.Of(int32(s.Weight)),
			},
		}},
	}}
	refs := slices.Map(s.Gateways, func(s string) gateway.ParentReference {
		ns, name, ok := strings.Cut(s, "/")
		if ok {
			return gateway.ParentReference{
				Namespace: ptr.Of(gateway.Namespace(ns)),
				Name:      gateway.ObjectName(name),
			}
		}
		return gateway.ParentReference{
			Name: gateway.ObjectName(name),
		}
	})
	return &gateway.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.App,
			Namespace: s.Namespace,
		},
		Spec: gateway.HTTPRouteSpec{
			CommonRouteSpec: gateway.CommonRouteSpec{
				ParentRefs: refs,
			},
			Hostnames: []gateway.Hostname{gateway.Hostname(s.App + ".example.com")},
			Rules:     routes,
		},
	}
}
