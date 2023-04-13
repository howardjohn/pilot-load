package app

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type ServiceSpec struct {
	App         string
	Namespace   string
	ClusterType model.ClusterType
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
	ports := []v1.ServicePort{
		{
			Name:       "http",
			Port:       80,
			TargetPort: intstr.FromInt(80),
		},
		{
			Name:       "https",
			Port:       443,
			TargetPort: intstr.FromInt(443),
		},
	}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.App,
			Namespace: p.Namespace,
		},
		Spec: v1.ServiceSpec{
			// TODO port customization
			Ports: ports,
			Type:  "ClusterIP",
		},
	}
	if s.Spec.ClusterType != model.Real {
		svc.Spec.Selector = map[string]string{
			"app": p.App,
		}
	}
	return svc
}
