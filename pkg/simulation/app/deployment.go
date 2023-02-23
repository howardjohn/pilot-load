package app

import (
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"istio.io/istio/pkg/ptr"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeploymentSpec struct {
	ServiceAccount string
	Replicas       int
	Node           string
	App            string
	Namespace      string
	PodType        model.PodType
	ClusterType    model.ClusterType
}

type Deployment struct {
	Spec *DeploymentSpec
}

var _ model.Simulation = &Deployment{}

func NewDeployment(s DeploymentSpec) *Deployment {
	return &Deployment{Spec: &s}
}

func (e *Deployment) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(e.getDeployment())
}

func (e *Deployment) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(e.getDeployment())
}

func (e *Deployment) getDeployment() *appsv1.Deployment {
	s := e.Spec
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.App,
			Namespace: s.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.Of(int32(s.Replicas)),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": s.App}},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": s.App},
				},
				Spec: v1.PodSpec{
					TerminationGracePeriodSeconds: ptr.Of(int64(0)),
					ServiceAccountName:            s.ServiceAccount,
					Containers: []v1.Container{{
						Name:  "app",
						Image: "fake",
					}},
					NodeSelector: map[string]string{
						"pilot-load.istio.io/node": "fake",
					},
					Tolerations: []v1.Toleration{{
						Key:      "pilot-load.istio.io/node",
						Operator: v1.TolerationOpExists,
						Effect:   v1.TaintEffectNoSchedule,
					}},
				},
			},
		},
	}
	return dep
}
