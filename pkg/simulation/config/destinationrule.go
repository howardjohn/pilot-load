package config

import (
	"fmt"

	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type DestinationRuleSpec struct {
	App           string
	Namespace     string
	Subsets       []string
	LbPolicyIndex int
}

type DestinationRule struct {
	Spec *DestinationRuleSpec
}

var _ model.Simulation = &DestinationRule{}

func NewDestinationRule(s DestinationRuleSpec) *DestinationRule {
	return &DestinationRule{Spec: &s}
}

func (v *DestinationRule) Run(ctx model.Context) (err error) {
	return kube.Apply(ctx.Client, v.getDestinationRule())
}

func (v *DestinationRule) Cleanup(ctx model.Context) error {
	return kube.Delete(ctx.Client, v.getDestinationRule())
}

func (v *DestinationRule) Refresh(ctx model.Context) (string, error) {
	v.Spec.LbPolicyIndex = (v.Spec.LbPolicyIndex + 1) % 3
	return fmt.Sprintf("%s/%s", v.Spec.Namespace, v.Spec.App), v.Run(ctx)
}

func (v *DestinationRule) getDestinationRule() *v1alpha3.DestinationRule {
	s := v.Spec
	subsets := []*networkingv1alpha3.Subset{}
	for _, ss := range s.Subsets {
		subsets = append(subsets, &networkingv1alpha3.Subset{Name: ss})
	}
	var lbPolicy networkingv1alpha3.LoadBalancerSettings_SimpleLB
	switch s.LbPolicyIndex {
	case 0:
		lbPolicy = networkingv1alpha3.LoadBalancerSettings_ROUND_ROBIN
	case 1:
		lbPolicy = networkingv1alpha3.LoadBalancerSettings_LEAST_REQUEST
	}
	return &v1alpha3.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.App,
			Namespace: s.Namespace,
		},
		Spec: networkingv1alpha3.DestinationRule{
			Host:    s.App,
			Subsets: subsets,
			TrafficPolicy: &networkingv1alpha3.TrafficPolicy{
				LoadBalancer: &networkingv1alpha3.LoadBalancerSettings{
					LbPolicy: &networkingv1alpha3.LoadBalancerSettings_Simple{
						Simple: lbPolicy,
					},
				},
			},
		},
	}
}
