package config

import (
	structpb "github.com/golang/protobuf/ptypes/struct"
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type EnvoyFilterSpec struct {
	App            string
	Namespace      string
	connectTimeout int
	APIScope       model.APIScope
}

type EnvoyFilter struct {
	Spec *EnvoyFilterSpec
}

var _ model.Simulation = &EnvoyFilter{}

func NewEnvoyFilter(s EnvoyFilterSpec) *EnvoyFilter {
	return &EnvoyFilter{Spec: &s}
}

func (v *EnvoyFilter) Refresh(ctx model.Context) error {
	v.Spec.connectTimeout = (v.Spec.connectTimeout + 1) % 10
	return v.Run(ctx)
}

func (v *EnvoyFilter) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getEnvoyFilter())
}

func (v *EnvoyFilter) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getEnvoyFilter())
}

func (v *EnvoyFilter) getEnvoyFilter() *v1alpha3.EnvoyFilter {
	s := v.Spec
	spec := networkingv1alpha3.EnvoyFilter{}

	name := s.Namespace
	applyTo := networkingv1alpha3.EnvoyFilter_NETWORK_FILTER
	context := networkingv1alpha3.EnvoyFilter_SIDECAR_OUTBOUND
	operation := networkingv1alpha3.EnvoyFilter_Patch_INSERT_AFTER

	// Apply different configurations at different levels
	if s.APIScope == model.Namespace {
		applyTo = networkingv1alpha3.EnvoyFilter_HTTP_FILTER
		context = networkingv1alpha3.EnvoyFilter_SIDECAR_INBOUND
		operation = networkingv1alpha3.EnvoyFilter_Patch_INSERT_BEFORE

	} else if s.APIScope == model.Application {
		name = s.App
		applyTo = networkingv1alpha3.EnvoyFilter_LISTENER
		context = networkingv1alpha3.EnvoyFilter_ANY
		operation = networkingv1alpha3.EnvoyFilter_Patch_INSERT_FIRST

		spec.WorkloadSelector = &networkingv1alpha3.WorkloadSelector{
			Labels: map[string]string{
				"app": s.App,
			},
		}
	}

	configPatches := []*networkingv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{}
	configPatch := networkingv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: applyTo,

		Match: &networkingv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch{
			Context: context,
		},

		Patch: &networkingv1alpha3.EnvoyFilter_Patch{
			Operation: operation,
			Value: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"name":            &structpb.Value{Kind: &structpb.Value_StringValue{"patch"}},
					"connect_timeout": &structpb.Value{Kind: &structpb.Value_NumberValue{float64(s.connectTimeout)}},
				},
			},
		},
	}
	configPatches = append(configPatches, &configPatch)
	spec.ConfigPatches = configPatches

	return &v1alpha3.EnvoyFilter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.Namespace,
		},
		Spec: spec,
	}
}
