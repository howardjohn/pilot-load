package config

import (
	securityv1beta1 "istio.io/api/security/v1beta1"
	typev1beta1 "istio.io/api/type/v1beta1"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type RequestAuthenticationSpec struct {
	App       string
	Namespace string
	APIScope  model.APIScope
}

type RequestAuthentication struct {
	Spec *RequestAuthenticationSpec
}

var _ model.Simulation = &RequestAuthentication{}

func NewRequestAuthentication(s RequestAuthenticationSpec) *RequestAuthentication {
	return &RequestAuthentication{Spec: &s}
}

func (v *RequestAuthentication) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getRequestAuthentication())
}

func (v *RequestAuthentication) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getRequestAuthentication())
}

func (v *RequestAuthentication) getRequestAuthentication() *v1beta1.RequestAuthentication {
	s := v.Spec
	name := s.Namespace
	spec := securityv1beta1.RequestAuthentication{}

	if s.APIScope == model.Application {
		name = s.App
		spec.Selector = &typev1beta1.WorkloadSelector{
			MatchLabels: map[string]string{
				"app": s.App,
			},
		}
	}
	spec = securityv1beta1.RequestAuthentication{
		JwtRules: []*securityv1beta1.JWTRule{
			{
				Issuer:  name + "-foo",
				JwksUri: "https://raw.githubusercontent.com/istio/istio/master/tests/common/jwt/jwks.json",
			},
			{
				Issuer:  name + "-bar",
				JwksUri: "https://raw.githubusercontent.com/istio/istio/master/tests/common/jwt/jwks.json",
			},
		},
	}
	return &v1beta1.RequestAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.Namespace,
		},
		Spec: spec,
	}
}
