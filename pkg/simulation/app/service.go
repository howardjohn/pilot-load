package app

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type ServiceSpec struct {
	App       string
	Namespace string
	IP        string
}

type Service struct {
	Spec *ServiceSpec
}

var _ model.Simulation = &Service{}

func NewService(s ServiceSpec) *Service {
	return &Service{Spec: &s}
}

func (s *Service) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(s.getService())
}

func (s *Service) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(s.getService())
}

func (s *Service) getService() *v1.Service {
	p := s.Spec
	return &v1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.App,
			Namespace: p.Namespace,
		},
		Spec: v1.ServiceSpec{
			// TODO port customization
			Ports: []v1.ServicePort{
				{
					Name: "http",
					Port: 80,
				},
			},
			Selector: map[string]string{
				"app": p.App,
			},
			ClusterIP: p.IP,
			Type:      "ClusterIp",
		},
	}
}
