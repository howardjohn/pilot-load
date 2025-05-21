package config

import (
	"fmt"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway "sigs.k8s.io/gateway-api/apis/v1beta1"

	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/ptr"
)

type KubeGatewaySpec struct {
	App       string
	Name      string
	Namespace string
	Waypoint  bool
}

type KubeGateway struct {
	Spec *KubeGatewaySpec
}

var _ model.Simulation = &KubeGateway{}

func NewKubeGateway(s KubeGatewaySpec) *KubeGateway {
	return &KubeGateway{Spec: &s}
}

func (v *KubeGateway) Run(ctx model.Context) (err error) {
	return kube.Apply(ctx.Client, v.getGateway())
}

func (v *KubeGateway) Cleanup(ctx model.Context) error {
	return kube.Delete(ctx.Client, v.getGateway())
}

func (v *KubeGateway) getGateway() *gateway.Gateway {
	var listeners []gateway.Listener
	var class string
	if v.Spec.Waypoint {
		class = constants.WaypointGatewayClassName
		listeners = []gateway.Listener{{
			Name:     "mesh",
			Port:     gateway.PortNumber(15008),
			Protocol: "HBONE",
		}}
	} else {
		class = "istio"
		listeners = []gateway.Listener{
			{
				Name:     "http",
				Port:     gateway.PortNumber(80),
				Protocol: "HTTP",
				Hostname: ptr.Of(gateway.Hostname("*.example.com")),
				AllowedRoutes: &gateway.AllowedRoutes{
					Namespaces: &gateway.RouteNamespaces{From: ptr.Of(gateway.FromNamespaces("All"))},
				},
			},
			{
				Name:     "https",
				Port:     gateway.PortNumber(443),
				Protocol: "HTTPS",
				Hostname: ptr.Of(gateway.Hostname("*.example.com")),
				AllowedRoutes: &gateway.AllowedRoutes{
					Namespaces: &gateway.RouteNamespaces{From: ptr.Of(gateway.FromNamespaces("All"))},
				},
				TLS: &gateway.GatewayTLSConfig{
					Mode: ptr.Of(gateway.TLSModeType("Terminate")),
					CertificateRefs: []gateway.SecretObjectReference{{
						Name:      gateway.ObjectName(v.Spec.App),
						Namespace: ptr.Of(gateway.Namespace(v.Spec.Namespace)),
					}},
				},
			},
		}
	}
	return &gateway.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Spec.Name,
			Namespace: v.Spec.Namespace,
		},
		Spec: gateway.GatewaySpec{
			Addresses: []gateway.GatewaySpecAddress{{
				Type:  ptr.Of(gateway.HostnameAddressType),
				Value: fmt.Sprintf("%s.%s.svc.cluster.local", v.Spec.App, v.Spec.Namespace),
			}},
			GatewayClassName: gateway.ObjectName(class),
			Listeners:        listeners,
		},
	}
}
