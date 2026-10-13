package cluster

import (
	"fmt"
	"time"

	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/sleep"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type KubernetesNamespaceSpec struct {
	Existing bool
	Name     string
	Labels   map[string]string
}

type KubernetesNamespace struct {
	Spec *KubernetesNamespaceSpec
}

var _ model.Simulation = &KubernetesNamespace{}

func NewKubernetesNamespace(s KubernetesNamespaceSpec) *KubernetesNamespace {
	return &KubernetesNamespace{Spec: &s}
}

func (n *KubernetesNamespace) Run(ctx model.Context) error {
	if n.Spec.Existing {
		_, err := ctx.Client.Kube().CoreV1().Namespaces().Get(ctx, n.Spec.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get existing namespace %q: %w", n.Spec.Name, err)
		}
		return nil
	}
	var err error
	// Retry in case it is still finalizing
	for range 100 {
		if err = kube.Apply(ctx.Client, n.getKubernetesNamespace()); err == nil {
			return nil
		}
		log.Warnf("namespace failed, retrying...: %v", err)
		if !sleep.UntilContext(ctx, time.Millisecond*500) {
			return err
		}
	}
	return err
}

func (n *KubernetesNamespace) Cleanup(ctx model.Context) error {
	if n.Spec.Existing || n.Spec.Name == "default" {
		return nil
	}
	if err := kube.Delete(ctx.Client, n.getKubernetesNamespace()); err != nil {
		return err
	}
	return nil
}

func (n *KubernetesNamespace) getKubernetesNamespace() *v1.Namespace {
	s := n.Spec
	return &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   s.Name,
			Labels: s.Labels,
		},
		Status: v1.NamespaceStatus{Phase: v1.NamespaceActive},
	}
}
