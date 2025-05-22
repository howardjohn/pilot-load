package config

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"text/template"

	"istio.io/istio/pkg/kube/controllers"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/reader"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

// Template inputs
const (
	RandNumber = "RandNumber"
	Rand       = "Rand"
	Namespace  = "Namespace"
	Name       = "Name"
)

type TemplatedSpec struct {
	Template *template.Template
	Config   map[string]any
	Refresh  *bool
}

type Templated struct {
	Spec        *TemplatedSpec
	Refreshable bool
}

var _ model.Simulation = &Templated{}

func NewTemplated(s TemplatedSpec) *Templated {
	setupConfig(&s)
	res := &Templated{Spec: &s}
	if s.Refresh != nil {
		res.Refreshable = *s.Refresh
	} else {
		res.Refreshable, _ = res.getDefaultRefresh()
	}
	return res
}

func setupConfig(s *TemplatedSpec) {
	s.Config[RandNumber] = rand.Intn(10000) + 1
	if b, f := s.Config[Rand]; f {
		s.Config[Rand] = !b.(bool)
	} else {
		s.Config[Rand] = false
	}
}

func (v *Templated) IsRefreshable() bool {
	return v.Refreshable
}

func (v *Templated) Refresh(ctx model.Context) (string, error) {
	setupConfig(v.Spec)
	v.Spec.Config[RandNumber] = rand.Intn(10000) + 1
	obj, err := v.getTemplated()
	if err != nil {
		return "", err
	}
	k := obj.GetObjectKind().GroupVersionKind().Kind
	name := k + "/" + obj.GetNamespace() + "/" + obj.GetName()
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

func (v *Templated) getDefaultRefresh() (bool, error) {
	var b bytes.Buffer
	if err := v.Spec.Template.Execute(&b, v.Spec.Config); err != nil {
		return false, err
	}
	if strings.Contains(b.String(), "#refresh=false") {
		return false, nil
	}
	if strings.Contains(b.String(), "#refresh=true") {
		return true, nil
	}
	return false, nil
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
