package app

import (
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ServiceAccountSpec struct {
	Namespace string
	Name      string
}

type ServiceAccount struct {
	Spec *ServiceAccountSpec
}

var _ model.Simulation = &ServiceAccount{}

func NewServiceAccount(s ServiceAccountSpec) *ServiceAccount {
	return &ServiceAccount{Spec: &s}
}

func (s *ServiceAccount) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(s.getServiceAccount())
}

func (s *ServiceAccount) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(s.getServiceAccount())
}

func (s *ServiceAccount) getServiceAccount() *v1.ServiceAccount {
	p := s.Spec
	return &v1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.Name,
			Namespace: p.Namespace,
		},
	}
}
