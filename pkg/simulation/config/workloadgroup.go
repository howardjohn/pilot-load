package config

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
)

type WorkloadGroupSpec struct {
	App       string
	Namespace string
}

type WorkloadGroup struct {
	Spec *WorkloadGroupSpec
}

var _ model.Simulation = &WorkloadGroup{}

func NewWorkloadGroup(s WorkloadGroupSpec) *WorkloadGroup {
	return &WorkloadGroup{Spec: &s}
}

func (v *WorkloadGroup) Run(ctx model.Context) (err error) {
	return kube.Apply(ctx.Client, v.getWorkloadGroup())
}

func (v *WorkloadGroup) Cleanup(ctx model.Context) error {
	return kube.Delete(ctx.Client, v.getWorkloadGroup())
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
				Ports: map[string]uint32{
					"http": 8080,
				},
				Labels: map[string]string{
					"app": s.App,
				},
			},
		},
	}
}
