package config

import (
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type ServiceEntrySpec struct {
	App       string
	AppType   model.AppType
	Namespace string
}

type ServiceEntry struct {
	Spec *ServiceEntrySpec
}

var _ model.Simulation = &ServiceEntry{}

func NewServiceEntry(s ServiceEntrySpec) *ServiceEntry {
	return &ServiceEntry{Spec: &s}
}

func (v *ServiceEntry) Run(ctx model.Context) (err error) {
	return kube.Apply(ctx.Client, v.getServiceEntry())
}

func (v *ServiceEntry) Cleanup(ctx model.Context) error {
	return kube.Delete(ctx.Client, v.getServiceEntry())
}

func (v *ServiceEntry) getServiceEntry() *v1alpha3.ServiceEntry {
	s := v.Spec
	spec := networkingv1alpha3.ServiceEntry{
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
		Resolution: networkingv1alpha3.ServiceEntry_DNS,
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
