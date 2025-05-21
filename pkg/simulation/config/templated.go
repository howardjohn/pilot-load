package config

import (
	"bytes"
	"fmt"
	"math/rand"
	"text/template"

	"istio.io/istio/pkg/kube/controllers"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/reader"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

// Template inputs
const (
	RandNumber = "RandNumber"
	Namespace  = "Namespace"
	Name       = "Name"
)

type TemplatedSpec struct {
	Template *template.Template
	Config   map[string]any
}

type Templated struct {
	Spec *TemplatedSpec
}

var _ model.Simulation = &Templated{}

func NewTemplated(s TemplatedSpec) *Templated {
	s.Config[RandNumber] = rand.Intn(10000) + 1
	return &Templated{Spec: &s}
}

func (v *Templated) Refresh(ctx model.Context) (string, error) {
	v.Spec.Config[RandNumber] = rand.Intn(10000) + 1
	obj, err := v.getTemplated()
	if err != nil {
		return "", err
	}
	name := obj.GetNamespace() + "/" + obj.GetName()
	return name, kube.ApplyRealSSA(ctx.Client, obj)
}

func (v *Templated) Run(ctx model.Context) (err error) {
	obj, err := v.getTemplated()
	if err != nil {
		return err
	}
	return kube.ApplyRealSSA(ctx.Client, obj)
}

func (v *Templated) Cleanup(ctx model.Context) error {
	obj, err := v.getTemplated()
	if err != nil {
		return err
	}
	return kube.Delete(ctx.Client, obj)
}

func (v *Templated) getTemplated() (controllers.Object, error) {
	var b bytes.Buffer
	if err := v.Spec.Template.Execute(&b, v.Spec.Config); err != nil {
		return nil, err
	}
	objs, err := reader.ParseYaml(&b)
	if err != nil {
		return nil, err
	}
	if len(objs) != 1 {
		return nil, fmt.Errorf("expected 1 object, got %d", len(objs))
	}
	objs[0].SetNamespace(v.Spec.Config[Namespace].(string))
	return objs[0], nil
}
