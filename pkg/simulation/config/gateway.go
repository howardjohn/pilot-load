package config

import (
	"istio.io/client-go/pkg/apis/networking/v1alpha3"

	networkingv1alpha3 "istio.io/api/networking/v1alpha3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
)

type GatewaySpec struct {
	App       string
	Namespace string
	Name      string
}

type Gateway struct {
	Spec *GatewaySpec
}

var _ model.Simulation = &Gateway{}

func NewGateway(s GatewaySpec) *Gateway {
	return &Gateway{Spec: &s}
}

func (v *Gateway) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getGateway())
}

func (v *Gateway) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getGateway())
}

func (v *Gateway) getGateway() *v1alpha3.Gateway {
	s := v.Spec
	return &v1alpha3.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.StringDefault(s.Name, s.App),
			Namespace: s.Namespace,
		},
		Spec: networkingv1alpha3.Gateway{
			Servers: []*networkingv1alpha3.Server{
				{
					Port: &networkingv1alpha3.Port{
						Number:   80,
						Name:     "http",
						Protocol: "HTTP",
					},
					Hosts: []string{s.App + ".example.com"},
				},
				{
					Port: &networkingv1alpha3.Port{
						Number:   443,
						Name:     "https",
						Protocol: "HTTPS",
					},
					Hosts: []string{s.App + ".example.com"},
					Tls: &networkingv1alpha3.ServerTLSSettings{
						HttpsRedirect: false,
						// TODO create a real cert, use simple
						Mode: networkingv1alpha3.ServerTLSSettings_ISTIO_MUTUAL,
					},
				},
			},
			Selector: map[string]string{
				"app": s.App,
			},
		},
	}
}
