package config

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
)

type GatewaySpec struct {
	App       string
	UID       string
	Namespace string
	Name      string
}

type Gateway struct {
	Spec *GatewaySpec
}

var _ model.Simulation = &Gateway{}

func NewGateway(s GatewaySpec) *Gateway {
	if s.UID == "" {
		s.UID = util.GenUID()
	}
	return &Gateway{Spec: &s}
}

func (v *Gateway) Run(ctx model.Context) (err error) {
	return kube.Apply(ctx.Client, v.getGateway())
}

func (v *Gateway) Cleanup(ctx model.Context) error {
	return kube.Delete(ctx.Client, v.getGateway())
}

func (v *Gateway) Name() string {
	s := v.Spec
	return fmt.Sprintf("%s-%s", util.StringDefault(s.Name, s.App), s.UID)
}

func (v *Gateway) getGateway() *v1alpha3.Gateway {
	n := v.Name()
	return &v1alpha3.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      n,
			Namespace: v.Spec.Namespace,
		},
		Spec: networkingv1alpha3.Gateway{
			Servers: []*networkingv1alpha3.Server{
				{
					Port: &networkingv1alpha3.Port{
						Number:   80,
						Name:     "http",
						Protocol: "HTTP",
					},
					Hosts: []string{"*.example.com"},
				},
				{
					Port: &networkingv1alpha3.Port{
						Number:   443,
						Name:     "https",
						Protocol: "HTTPS",
					},
					Hosts: []string{"*.example.com"},
					Tls: &networkingv1alpha3.ServerTLSSettings{
						HttpsRedirect:  false,
						Mode:           networkingv1alpha3.ServerTLSSettings_SIMPLE,
						CredentialName: n,
					},
				},
			},
			Selector: map[string]string{
				"app": v.Spec.App,
			},
		},
	}
}
