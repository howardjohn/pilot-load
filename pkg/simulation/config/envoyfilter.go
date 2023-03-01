package config

import (
	"math/rand"

	structpb "github.com/golang/protobuf/ptypes/struct"
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type EnvoyFilterSpec struct {
	App            string
	Namespace      string
	randomSampling int
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
	v.Spec.randomSampling = rand.Intn(100) + 1
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
	if s.APIScope == model.Application {
		name = s.App
		spec.WorkloadSelector = &networkingv1alpha3.WorkloadSelector{
			Labels: map[string]string{
				"app": s.App,
			},
		}
	}

	configPatches := []*networkingv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{}
	configPatch := networkingv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: networkingv1alpha3.EnvoyFilter_NETWORK_FILTER,

		Match: &networkingv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch{
			Context: networkingv1alpha3.EnvoyFilter_ANY,
			ObjectTypes: &networkingv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
				Listener: &networkingv1alpha3.EnvoyFilter_ListenerMatch{
					FilterChain: &networkingv1alpha3.EnvoyFilter_ListenerMatch_FilterChainMatch{
						Filter: &networkingv1alpha3.EnvoyFilter_ListenerMatch_FilterMatch{
							Name: "envoy.http_connection_manager",
						},
					},
				},
			},
		},

		Patch: &networkingv1alpha3.EnvoyFilter_Patch{
			Operation: networkingv1alpha3.EnvoyFilter_Patch_MERGE,
			Value: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"typed_config": {Kind: &structpb.Value_StructValue{
						StructValue: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"@type": {Kind: &structpb.Value_StringValue{StringValue: "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"}},
								"tracing": {Kind: &structpb.Value_StructValue{
									StructValue: &structpb.Struct{
										Fields: map[string]*structpb.Value{
											"random_sampling": {Kind: &structpb.Value_StructValue{
												StructValue: &structpb.Struct{
													Fields: map[string]*structpb.Value{
														"value": {Kind: &structpb.Value_NumberValue{NumberValue: float64(s.randomSampling)}},
													},
												},
											},
											},
										},
									},
								},
								},
							},
						},
					}},
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
