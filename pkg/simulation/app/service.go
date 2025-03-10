package app

import (
	"istio.io/istio/pkg/maps"
	"istio.io/istio/pkg/ptr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type ServiceSpec struct {
	App       string
	Namespace string
	Waypoint  bool
	Labels    map[string]string
}

type Service struct {
	Spec *ServiceSpec
}

var _ model.Simulation = &Service{}

func NewService(s ServiceSpec) *Service {
	return &Service{Spec: &s}
}

func (s *Service) Run(ctx model.Context) (err error) {
	return kube.Apply(ctx.Client, s.getService())
}

func (s *Service) Cleanup(ctx model.Context) error {
	return kube.Delete(ctx.Client, s.getService())
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
	lbls := s.Spec.Labels
	if s.Spec.Waypoint {
		lbls = maps.Clone(lbls)
		if lbls == nil {
			lbls = map[string]string{}
		}
		// Make sure we don't mark the waypoint as having a waypoint
		lbls["gateway.istio.io/managed"] = "istio.io-mesh-controller"
		ports = []v1.ServicePort{
			{
				Name:        "mesh",
				AppProtocol: ptr.Of("all"),
				Port:        15008,
			},
		}
	}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.App,
			Namespace: p.Namespace,
			Labels:    lbls,
		},
		Spec: v1.ServiceSpec{
			// TODO port customization
			Ports: ports,
			Type:  "ClusterIP",
		},
	}
	svc.Spec.Selector = map[string]string{
		"app": p.App,
	}
	return svc
}
