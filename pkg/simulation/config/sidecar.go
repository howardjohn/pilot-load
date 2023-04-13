package config

import (
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type SidecarSpec struct {
	App       string
	Namespace string
	ModeIndex int
	APIScope  model.APIScope
}

type Sidecar struct {
	Spec *SidecarSpec
}

var _ model.Simulation = &Sidecar{}

func NewSidecar(s SidecarSpec) *Sidecar {
	return &Sidecar{Spec: &s}
}

func (v *Sidecar) Refresh(ctx model.Context) error {
	v.Spec.ModeIndex = (v.Spec.ModeIndex + 1) % 2
	return v.Run(ctx)
}

func (v *Sidecar) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getSidecar())
}

func (v *Sidecar) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getSidecar())
}

func (v *Sidecar) getSidecar() *v1alpha3.Sidecar {
	s := v.Spec
	spec := networkingv1alpha3.Sidecar{}
	name := s.Namespace

	spec.OutboundTrafficPolicy = &networkingv1alpha3.OutboundTrafficPolicy{
		Mode: networkingv1alpha3.OutboundTrafficPolicy_Mode(s.ModeIndex),
	}

	if s.APIScope == model.Application {
		name = s.App
		spec.WorkloadSelector = &networkingv1alpha3.WorkloadSelector{
			Labels: map[string]string{
				"app": v.Spec.App,
			},
		}
	}
	spec.Egress = []*networkingv1alpha3.IstioEgressListener{{
		Hosts: []string{"./*"},
	}}

	return &v1alpha3.Sidecar{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.Namespace,
		},
		Spec: spec,
	}
}
