package app

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type NamespaceSpec struct {
	Name string
}

type Namespace struct {
	Spec *NamespaceSpec
}

var _ model.Simulation = &Namespace{}

func NewNamespace(s NamespaceSpec) *Namespace {
	return &Namespace{Spec: &s}
}

func (n *Namespace) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(n.getNamespace())
}

func (n *Namespace) Cleanup(ctx model.Context) error {
	// TODO force remove finalizers
	return ctx.Client.Delete(n.getNamespace())
}

func (n *Namespace) getNamespace() *v1.Namespace {
	s := n.Spec
	return &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.Name,
			Labels: map[string]string{
				"istio-injection": "enabled",
			},
		},
		Status: v1.NamespaceStatus{Phase: v1.NamespaceActive},
	}
}
