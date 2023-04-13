package config

import (
	securityv1beta1 "istio.io/api/security/v1beta1"
	typev1beta1 "istio.io/api/type/v1beta1"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type PeerAuthenticationSpec struct {
	App       string
	Namespace string
	ModeIndex int
	APIScope  model.APIScope
}

type PeerAuthentication struct {
	Spec *PeerAuthenticationSpec
}

var _ model.Simulation = &PeerAuthentication{}

func NewPeerAuthentication(s PeerAuthenticationSpec) *PeerAuthentication {
	return &PeerAuthentication{Spec: &s}
}

func (v *PeerAuthentication) Refresh(ctx model.Context) error {
	v.Spec.ModeIndex = (v.Spec.ModeIndex + 1) % 4
	return v.Run(ctx)
}

func (v *PeerAuthentication) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getPeerAuthentication())
}

func (v *PeerAuthentication) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getPeerAuthentication())
}

func (v *PeerAuthentication) getPeerAuthentication() *v1beta1.PeerAuthentication {
	s := v.Spec
	name := s.Namespace
	spec := securityv1beta1.PeerAuthentication{
		Mtls: &securityv1beta1.PeerAuthentication_MutualTLS{
			Mode: securityv1beta1.PeerAuthentication_MutualTLS_Mode(s.ModeIndex),
		},
	}

	if s.APIScope == model.Application {
		name = s.App
		spec.Selector = &typev1beta1.WorkloadSelector{
			MatchLabels: map[string]string{
				"app": s.App,
			},
		}
	}
	return &v1beta1.PeerAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.Namespace,
		},
		Spec: spec,
	}
}
