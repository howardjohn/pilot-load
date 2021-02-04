package cluster

import (
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type KubernetesNamespaceSpec struct {
	Name string
	// If true, will treat this as a real cluster and not attempt to force cleanup
	RealCluster bool
}

type KubernetesNamespace struct {
	Spec *KubernetesNamespaceSpec
}

var _ model.Simulation = &KubernetesNamespace{}

func NewKubernetesNamespace(s KubernetesNamespaceSpec) *KubernetesNamespace {
	return &KubernetesNamespace{Spec: &s}
}

func (n *KubernetesNamespace) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(n.getKubernetesNamespace())
}

func (n *KubernetesNamespace) Cleanup(ctx model.Context) error {
	if err := ctx.Client.Delete(n.getKubernetesNamespace()); err != nil {
		return err
	}
	if n.Spec.RealCluster {
		return ctx.Client.Finalize(n.getKubernetesNamespace())
	}
	return nil
}

func (n *KubernetesNamespace) getKubernetesNamespace() *v1.Namespace {
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
