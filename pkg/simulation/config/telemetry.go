package config

import (
	"math/rand"

	"google.golang.org/protobuf/types/known/wrapperspb"
	telemetryv1alpha1 "istio.io/api/telemetry/v1alpha1"
	typev1beta1 "istio.io/api/type/v1beta1"
	"istio.io/client-go/pkg/apis/telemetry/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type TelemetrySpec struct {
	App                      string
	Namespace                string
	RandomSamplingPercentage int
	APIScope                 model.APIScope
}

type Telemetry struct {
	Spec *TelemetrySpec
}

var _ model.Simulation = &Telemetry{}

func NewTelemetry(s TelemetrySpec) *Telemetry {
	return &Telemetry{Spec: &s}
}

func (v *Telemetry) Refresh(ctx model.Context) error {
	v.Spec.RandomSamplingPercentage = rand.Intn(100) + 1
	return v.Run(ctx)
}

func (v *Telemetry) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.getTelemetry())
}

func (v *Telemetry) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.getTelemetry())
}

func (v *Telemetry) getTelemetry() *v1alpha1.Telemetry {
	s := v.Spec
	name := s.Namespace
	spec := telemetryv1alpha1.Telemetry{
		Tracing: []*telemetryv1alpha1.Tracing{
			{
				RandomSamplingPercentage: wrapperspb.Double(float64(s.RandomSamplingPercentage)),
			},
		},
	}

	if s.APIScope == model.Application {
		name = s.App
		spec.Selector = &typev1beta1.WorkloadSelector{
			MatchLabels: map[string]string{
				"app": s.App,
			},
		}
	}
	return &v1alpha1.Telemetry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.Namespace,
		},
		Spec: spec,
	}
}
