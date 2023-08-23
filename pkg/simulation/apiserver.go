package simulation

import (
	"time"

	"istio.io/istio/pkg/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type ApiServerSimulation struct{}

func (a ApiServerSimulation) Run(ctx model.Context) error {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: v1.PodSpec{
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
	}
	image := []string{"bar", "baz"}
	requests := 0
	totalLatency := time.Second * 0
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			requests++
			t0 := time.Now()
			pod.Spec.Containers[0].Image = image[requests%2]
			if err := kube.Apply(ctx.Client, pod); err != nil {
				return err
			}
			latency := time.Since(t0)
			totalLatency += latency
			log.Infof("latency: %v average: %v request: %v", latency, totalLatency/time.Duration(requests), requests)
		}
	}
}

func (a ApiServerSimulation) Cleanup(ctx model.Context) error {
	return kube.Delete(ctx.Client, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
	})
}

var _ model.Simulation = &ApiServerSimulation{}
