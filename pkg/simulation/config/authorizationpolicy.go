package config

import (
	securityv1beta1 "istio.io/api/security/v1beta1"
	typev1beta1 "istio.io/api/type/v1beta1"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type AuthorizationPolicySpec struct {
	App              string
	Namespace        string
	HttpMethodsIndex int
	APIScope         model.APIScope
}

type AuthorizationPolicy struct {
	Spec *AuthorizationPolicySpec
}

var _ model.Simulation = &AuthorizationPolicy{}

func NewAuthorizationPolicy(s AuthorizationPolicySpec) *AuthorizationPolicy {
	return &AuthorizationPolicy{Spec: &s}
}

func (v *AuthorizationPolicy) Refresh(ctx model.Context) error {
	v.Spec.HttpMethodsIndex = (v.Spec.HttpMethodsIndex + 1) % 4
	return v.Run(ctx)
}

func (v *AuthorizationPolicy) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getAuthorizationPolicy())
}

func (v *AuthorizationPolicy) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getAuthorizationPolicy())
}

func (v *AuthorizationPolicy) getAuthorizationPolicy() *v1beta1.AuthorizationPolicy {
	s := v.Spec
	name := s.Namespace
	var method string

	switch s.HttpMethodsIndex {
	case 0:
		method = "POST"
	case 1:
		method = "GET"
	case 2:
		method = "PUT"
	case 3:
		method = "DELETE"

	}

	spec := securityv1beta1.AuthorizationPolicy{}
	if s.APIScope == model.Application {
		name = s.App
		spec.Selector = &typev1beta1.WorkloadSelector{
			MatchLabels: map[string]string{
				"app": s.App,
			},
		}
	}

	spec = securityv1beta1.AuthorizationPolicy{
		Rules: []*securityv1beta1.Rule{
			{
				From: []*securityv1beta1.Rule_From{
					{
						Source: &securityv1beta1.Source{
							RequestPrincipals: []string{
								name + "-foo/*",
							},
						},
					},
				},
				To: []*securityv1beta1.Rule_To{
					{
						Operation: &securityv1beta1.Operation{
							Methods: []string{method},
							Hosts: []string{
								name + ".example.com",
							},
						},
					},
				},
			},
			{
				From: []*securityv1beta1.Rule_From{
					{
						Source: &securityv1beta1.Source{
							RequestPrincipals: []string{
								name + "-bar/*",
							},
						},
					},
				},
				To: []*securityv1beta1.Rule_To{
					{
						Operation: &securityv1beta1.Operation{
							Methods: []string{method},
							Hosts: []string{
								name + ".another-host.com",
							},
						},
					},
				},
			},
		},
	}

	return &v1beta1.AuthorizationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.Namespace,
		},
		Spec: spec,
	}
}
