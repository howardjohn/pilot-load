package config

import (
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type ServiceEntrySpec struct {
	App               string
	AppType           model.AppType
	Namespace         string
	SEResolutionIndex int
}

type ServiceEntry struct {
	Spec *ServiceEntrySpec
}

var _ model.Simulation = &ServiceEntry{}

func NewServiceEntry(s ServiceEntrySpec) *ServiceEntry {
	return &ServiceEntry{Spec: &s}
}

func (v *ServiceEntry) Refresh(ctx model.Context) error {
	v.Spec.SEResolutionIndex = (v.Spec.SEResolutionIndex + 1) % 3
	return v.Run(ctx)
}

func (v *ServiceEntry) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getServiceEntry())
}

func (v *ServiceEntry) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getServiceEntry())
}

func (v *ServiceEntry) getServiceEntry() *v1alpha3.ServiceEntry {

	s := v.Spec
	spec := networkingv1alpha3.ServiceEntry{}

	var seResolution networkingv1alpha3.ServiceEntry_Resolution
	switch s.SEResolutionIndex {
	case 0:
		seResolution = networkingv1alpha3.ServiceEntry_STATIC
	case 1:
		seResolution = networkingv1alpha3.ServiceEntry_DNS
	case 2:
		seResolution = networkingv1alpha3.ServiceEntry_DNS_ROUND_ROBIN
	}

	spec = networkingv1alpha3.ServiceEntry{
		Hosts:    []string{s.App},
		Location: networkingv1alpha3.ServiceEntry_MESH_INTERNAL,
		Ports: []*networkingv1alpha3.ServicePort{
			{
				Number:     80,
				Name:       "http",
				Protocol:   "HTTP",
				TargetPort: 8080,
			},
		},
		Resolution: seResolution,
	}

	if s.AppType == model.ExternalType {
		spec.Location = networkingv1alpha3.ServiceEntry_MESH_EXTERNAL
	} else {
		// workload selector is to link workload entry and service entry
		spec.WorkloadSelector = &networkingv1alpha3.WorkloadSelector{
			Labels: map[string]string{
				"app": s.App,
			},
		}
	}

	return &v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.App,
			Namespace: s.Namespace,
		},
		Spec: spec,
	}
}
