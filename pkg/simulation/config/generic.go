package config

import (
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type Generic struct {
	Spec runtime.Object
}

var _ model.Simulation = &Generic{}

func NewGeneric(s runtime.Object) *Generic {
	return &Generic{Spec: s}
}

func (v *Generic) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(v.Spec)
}

func (v *Generic) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(v.Spec)
}
