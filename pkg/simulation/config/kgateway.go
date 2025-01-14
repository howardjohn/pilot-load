package config

import (
	"fmt"

	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/ptr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type KubeGatewaySpec struct {
	App       string
	Namespace string
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
	return &gateway.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Spec.App,
			Namespace: v.Spec.Namespace,
		},
		Spec: gateway.GatewaySpec{
			Addresses: []gateway.GatewayAddress{{
				Type:  ptr.Of(gateway.HostnameAddressType),
				Value: fmt.Sprintf("%s.%s.svc.cluster.local", v.Spec.App, v.Spec.Namespace),
			}},
			GatewayClassName: constants.WaypointGatewayClassName,
			Listeners: []gateway.Listener{{
				Name:     "mesh",
				Port:     gateway.PortNumber(15008),
				Protocol: "HBONE",
			}},
		},
	}
}
