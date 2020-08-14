package config

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"

	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
)

type SidecarSpec struct {
	Namespace string
}

type Sidecar struct {
	Spec *SidecarSpec
}

var _ model.Simulation = &Sidecar{}

func NewSidecar(s SidecarSpec) *Sidecar {
	return &Sidecar{Spec: &s}
}

func (v *Sidecar) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getSidecar())
}

func (v *Sidecar) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getSidecar())
}

func (v *Sidecar) getSidecar() *v1alpha3.Sidecar {
	s := v.Spec
	return &v1alpha3.Sidecar{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scope-namespace",
			Namespace: s.Namespace,
		},
		Spec: networkingv1alpha3.Sidecar{
			Egress: []*networkingv1alpha3.IstioEgressListener{{
				Hosts: []string{"./*"},
			}},
		},
	}
}
