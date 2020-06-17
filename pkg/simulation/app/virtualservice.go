package app

import (
	"istio.io/client-go/pkg/apis/networking/v1alpha3"

	networkingv1alpha3 "istio.io/api/networking/v1alpha3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type VirtualServiceSpec struct {
	App       string
	Namespace string
}

type VirtualService struct {
	Spec *VirtualServiceSpec
}

var _ model.Simulation = &VirtualService{}

func NewVirtualService(s VirtualServiceSpec) *VirtualService {
	return &VirtualService{Spec: &s}
}

func (v *VirtualService) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getVirtualService())
}

func (v *VirtualService) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getVirtualService())
}

func (v *VirtualService) getVirtualService() *v1alpha3.VirtualService {
	s := v.Spec
	return &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.App,
			Namespace: s.Namespace,
		},
		Spec: networkingv1alpha3.VirtualService{
			Hosts:    []string{s.App},
			Gateways: nil,
			Http: []*networkingv1alpha3.HTTPRoute{
				{
					Route: []*networkingv1alpha3.HTTPRouteDestination{
						{
							Destination: &networkingv1alpha3.Destination{
								// TODO
								Host: "productpage",
							},
						},
					},
				},
			},
		},
	}
}
