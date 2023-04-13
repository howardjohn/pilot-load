package config

import (
	"math/rand"

	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type SubsetSpec struct {
	Name   string
	Weight int
}

type VirtualServiceSpec struct {
	App       string
	Namespace string
	Gateways  []string
	Subsets   []SubsetSpec
}

type VirtualService struct {
	Spec *VirtualServiceSpec
}

var (
	_ model.Simulation            = &VirtualService{}
	_ model.RefreshableSimulation = &VirtualService{}
)

func NewVirtualService(s VirtualServiceSpec) *VirtualService {
	return &VirtualService{Spec: &s}
}

func (v *VirtualService) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getVirtualService())
}

func (v *VirtualService) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getVirtualService())
}

func getRandomWeights(size int) []int {
	remainingWeight := 100
	var weights []int
	for i := 0; i < size-1; i++ {
		if remainingWeight == 0 {
			weights = append(weights, 0)
		} else {
			weight := rand.Intn(remainingWeight + 1)
			remainingWeight -= weight
			weights = append(weights, weight)
		}
	}
	weights = append(weights, remainingWeight)
	return weights
}

func (v *VirtualService) Refresh(ctx model.Context) error {
	for i, w := range getRandomWeights(len(v.Spec.Subsets)) {
		ss := v.Spec.Subsets[i]
		ss.Weight = w
		v.Spec.Subsets[i] = ss
	}
	return v.Run(ctx)
}

func (v *VirtualService) getVirtualService() *v1alpha3.VirtualService {
	s := v.Spec
	routes := []*networkingv1alpha3.HTTPRouteDestination{}
	for _, ss := range s.Subsets {
		routes = append(routes, &networkingv1alpha3.HTTPRouteDestination{
			Weight: int32(ss.Weight),
			Destination: &networkingv1alpha3.Destination{
				Host:   s.App,
				Subset: ss.Name,
				Port:   &networkingv1alpha3.PortSelector{Number: 80},
			},
		})
	}
	return &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.App,
			Namespace: s.Namespace,
		},
		Spec: networkingv1alpha3.VirtualService{
			Hosts:    []string{s.App + ".example.com"},
			Gateways: s.Gateways,
			Http: []*networkingv1alpha3.HTTPRoute{
				{
					Route: routes,
				},
			},
		},
	}
}
