package simulation

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/howardjohn/pilot-load/pkg/simulation/cluster"
	"github.com/howardjohn/pilot-load/pkg/simulation/gateway"
	"github.com/howardjohn/pilot-load/pkg/simulation/impersonate"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/monitoring"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"
)

// Load testing api-server
func ApiServer(a model.Args) error {
	if err := ExecuteSimulations(a, &ApiServerSimulation{}); err != nil {
		return fmt.Errorf("error executing: %v", err)
	}
	return nil
}

// Load testing pod startup
func PodStartup(a model.Args) error {
	if err := ExecuteSimulations(a, &PodStartupSimulation{a.StartupConfig}); err != nil {
		return fmt.Errorf("error executing: %v", err)
	}
	return nil
}

func GatewayProber(a model.Args) error {
	sim := gateway.NewSimulation(gateway.ProberSpec{
		Replicas:       a.ProberConfig.Replicas,
		Delay:          a.ProberConfig.Delay,
		DelayThreshold: a.ProberConfig.DelayThreshold,
		Address:        a.ProberConfig.GatewayAddress,
	})
	if err := ExecuteSimulations(a, sim); err != nil {
		return fmt.Errorf("error executing: %v", err)
	}
	return nil
}

func Impersonate(a model.Args) error {
	sim := impersonate.NewSimulation(impersonate.ImpersonateSpec{
		Selector: model.Selector(a.ImpersonateConfig.Selector),
		Replicas: a.ImpersonateConfig.Replicas,
		Delay:    a.ImpersonateConfig.Delay,
	})
	if err := ExecuteSimulations(a, sim); err != nil {
		return fmt.Errorf("error executing: %v", err)
	}
	return nil
}

func Determinism(a model.Args) error {
	sim := &DeterministicSimulation{}
	if err := ExecuteSimulations(a, sim); err != nil {
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
	opts := a.Auth.GrpcOptions("default", "default")

	for i := 0; i < count; i++ {
		sims = append(sims, &xds.Simulation{
			Namespace: "default",
			Name:      "adsc",
			IP:        util.GetIP(),
			// TODO: multicluster
			Cluster:  "Kubernetes",
			GrpcOpts: opts,
		})
	}
	return ExecuteSimulations(a, model.AggregateSimulation{Simulations: sims})
}

func ExecuteSimulations(a model.Args, simulation model.Simulation) error {
	ctx, cancel := context.WithCancel(context.Background())
	go captureTermination(ctx, cancel)
	defer cancel()
	go monitoring.StartMonitoring(ctx, 8765)
	simulationContext := model.Context{ctx, a, a.Client, cancel}
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
