package app

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"
)

type PodSpec struct {
	ServiceAccount string
	Node           string
	App            string
	Namespace      string
	UID            string
	IP             string
}

type Pod struct {
	Spec *PodSpec
}

var _ model.Simulation = &Pod{}

func NewPod(s PodSpec) *Pod {
	if s.UID == "" {
		s.UID = util.GenUID()
	}
	if s.IP == "" {
		s.IP = util.GetIP()
	}
	return &Pod{
		Spec: &s,
	}
}

func (p *Pod) Run(ctx model.Context) (err error) {
	pod := p.getPod()
	// TODO apply gets pod stuck in pending state.. figure out how to force Running
	if err = ctx.Client.Apply(pod); err != nil {
		return fmt.Errorf("failed to apply config: %v", err)
	}
	return xds.XdsSimulation{
		Labels:    pod.Labels,
		Namespace: pod.Namespace,
		Name:      pod.Name,
		IP:        p.Spec.IP,
		// TODO: multicluster
		Cluster: "pilot-load",
	}.Run(ctx)
}

func (p *Pod) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(p.getPod())
}

func (p *Pod) getPod() *v1.Pod {
	s := p.Spec
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", s.App, s.UID),
			Namespace: s.Namespace,
			Labels: map[string]string{
				"app": s.App,
			},
		},
		Spec: v1.PodSpec{
			Volumes: nil,
			InitContainers: []v1.Container{{
				Name:  "istio-init",
				Image: "istio/proxyv2",
			}},
			Containers: []v1.Container{{
				Name:  "app",
				Image: "app",
			}, {
				Name:  "istio-proxy",
				Image: "istio/proxyv2",
			}},
		},
		Status: v1.PodStatus{
			Phase:      v1.PodRunning,
			Conditions: nil,
			PodIP:      s.IP,
			PodIPs:     []v1.PodIP{{s.IP}},
		},
	}
}
