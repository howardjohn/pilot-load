package config

import (
	"istio.io/istio/pkg/kube/controllers"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type Generic[T controllers.Object] struct {
	Spec T
}

var _ model.Simulation = &Generic[controllers.Object]{}

func NewGeneric[T controllers.Object](s T) *Generic[T] {
	return &Generic[T]{Spec: s}
}

func (v *Generic[T]) Run(ctx model.Context) (err error) {
	return kube.Apply(ctx.Client, v.Spec)
}

func (v *Generic[T]) Cleanup(ctx model.Context) error {
	return kube.Delete(ctx.Client, v.Spec)
}
