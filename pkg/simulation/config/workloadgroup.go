package config

import (
	"math/rand"

	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type WorkloadGroupSpec struct {
	App       string
	Namespace string
	Weight    int
}

type WorkloadGroup struct {
	Spec *WorkloadGroupSpec
}

var _ model.Simulation = &WorkloadGroup{}

func NewWorkloadGroup(s WorkloadGroupSpec) *WorkloadGroup {
	return &WorkloadGroup{Spec: &s}
}

func (v *WorkloadGroup) Refresh(ctx model.Context) error {
	v.Spec.Weight = rand.Intn(100)
	return v.Run(ctx)
}

func (v *WorkloadGroup) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getWorkloadGroup())
}

func (v *WorkloadGroup) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getWorkloadGroup())
}

func (v *WorkloadGroup) getWorkloadGroup() *v1alpha3.WorkloadGroup {
	s := v.Spec
	return &v1alpha3.WorkloadGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.App,
			Namespace: s.Namespace,
		},
		Spec: networkingv1alpha3.WorkloadGroup{
			Metadata: &networkingv1alpha3.WorkloadGroup_ObjectMeta{
				Labels: map[string]string{
					"app": s.App,
				},
			},
			Template: &networkingv1alpha3.WorkloadEntry{
				Address: "8.8.8.8",
				Weight:  uint32(s.Weight),
				Labels: map[string]string{
					"app": s.App,
				},
			},
		},
	}
}
