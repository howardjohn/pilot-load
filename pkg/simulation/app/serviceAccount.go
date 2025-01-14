package app

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
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

func (s *ServiceAccount) Run(ctx model.Context) (err error) {
	return IgnoreExists(kube.Apply(ctx.Client, s.getServiceAccount()))
}

func IgnoreExists(err error) error {
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (s *ServiceAccount) Cleanup(ctx model.Context) error {
	return kube.Delete(ctx.Client, s.getServiceAccount())
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
