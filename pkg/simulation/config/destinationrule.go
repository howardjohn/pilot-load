package config

import (
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
)

type DestinationRuleSpec struct {
	App       string
	Namespace string
	Subsets   []string
}

type DestinationRule struct {
	Spec *DestinationRuleSpec
}

var _ model.Simulation = &DestinationRule{}

func NewDestinationRule(s DestinationRuleSpec) *DestinationRule {
	return &DestinationRule{Spec: &s}
}

func (v *DestinationRule) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getDestinationRule())
}

func (v *DestinationRule) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getDestinationRule())
}

func (v *DestinationRule) getDestinationRule() *v1alpha3.DestinationRule {
	s := v.Spec
	subsets := []*networkingv1alpha3.Subset{}
	for _, ss := range s.Subsets {
		subsets = append(subsets, &networkingv1alpha3.Subset{Name: ss})
	}
	return &v1alpha3.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.App,
			Namespace: s.Namespace,
		},
		Spec: networkingv1alpha3.DestinationRule{
			Host:    s.App,
			Subsets: subsets,
		},
	}
}
