package config

import (
	"math/rand"

	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
)

type WorkloadEntrySpec struct {
	App       string
	Namespace string
	Weight    int
}

type WorkloadEntry struct {
	Spec *WorkloadEntrySpec
}

var _ model.Simulation = &WorkloadEntry{}

func NewWorkloadEntry(s WorkloadEntrySpec) *WorkloadEntry {
	return &WorkloadEntry{Spec: &s}
}

func (v *WorkloadEntry) Refresh(ctx model.Context) error {
	v.Spec.Weight = rand.Intn(100)
	return v.Run(ctx)
}

func (v *WorkloadEntry) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getWorkloadEntry())
}

func (v *WorkloadEntry) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getWorkloadEntry())
}

func (v *WorkloadEntry) getWorkloadEntry() *v1alpha3.WorkloadEntry {
	s := v.Spec
	return &v1alpha3.WorkloadEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.App,
			Namespace: s.Namespace,
		},
		Spec: networkingv1alpha3.WorkloadEntry{
			Address: util.GetIP(),
			Weight:  uint32(s.Weight),
			Labels: map[string]string{
				"app": s.App,
			},
		},
	}
}
