package simulation

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/cluster"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/monitoring"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"

	"istio.io/pkg/log"
)

type ApiServerSimulation struct {
}

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
			if err := ctx.Client.Apply(pod); err != nil {
				return err
			}
			latency := time.Since(t0)
			totalLatency += latency
			log.Infof("latency: %v average: %v request: %v", latency, totalLatency/time.Duration(requests), requests)
		}
	}
}

func (a ApiServerSimulation) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(&v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		}})
}

var _ model.Simulation = &ApiServerSimulation{}

// Load testing api-server
func ApiServer(a model.Args) error {
	if err := ExecuteSimulations(a, &ApiServerSimulation{}); err != nil {
		return fmt.Errorf("error executing: %v", err)
	}
	return nil
}

func Cluster(a model.Args) error {
	sim := cluster.NewCluster(cluster.ClusterSpec{a.ClusterConfig})
	if err := ExecuteSimulations(a, sim); err != nil {
		return fmt.Errorf("error executing: %v", err)
	}
	return nil
}

func Adsc(a model.Args) error {
	sims := []model.Simulation{}
	count := a.AdsConfig.Count
	if count == 0 {
		count = 1
	}
	for i := 0; i < count; i++ {
		sims = append(sims, &xds.Simulation{
			Namespace: "default",
			Name:      "adsc",
			IP:        util.GetIP(),
			// TODO: multicluster
			Cluster: "Kubernetes",
		})
	}
	return ExecuteSimulations(a, model.AggregateSimulation{Simulations: sims})
}

func ExecuteSimulations(a model.Args, simulation model.Simulation) error {
	cl, err := kube.NewClient(a.KubeConfig, a.Qps)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	go captureTermination(ctx, cancel)
	defer cancel()
	go monitoring.StartMonitoring(ctx, 8765)
	simulationContext := model.Context{ctx, a, cl}
	if err := simulation.Run(simulationContext); err != nil {
		return err
	}
	<-ctx.Done()
	return simulation.Cleanup(simulationContext)
}

func captureTermination(ctx context.Context, cancel context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	defer func() {
		signal.Stop(c)
	}()
	select {
	case <-c:
		cancel()
	case <-ctx.Done():
	}
}
