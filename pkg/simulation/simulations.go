package simulation

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/howardjohn/pilot-load/pkg/simulation/cluster"
	"github.com/howardjohn/pilot-load/pkg/simulation/dump"
	"github.com/howardjohn/pilot-load/pkg/simulation/gateway"
	"github.com/howardjohn/pilot-load/pkg/simulation/impersonate"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/monitoring"
	"github.com/howardjohn/pilot-load/pkg/simulation/reproduce"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"

	"istio.io/pkg/log"
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

func Reproduce(a model.Args) error {
	sim := reproduce.NewSimulation(reproduce.ReproduceSpec{
		Delay:      a.ReproduceConfig.Delay,
		ConfigFile: a.ReproduceConfig.ConfigFile,
		ConfigOnly: a.ReproduceConfig.ConfigOnly,
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
	sim := cluster.NewCluster(cluster.ClusterSpec{Config: a.ClusterConfig})
	if err := ExecuteSimulations(a, sim); err != nil {
		return fmt.Errorf("error executing: %v", err)
	}
	return nil
}

func Dump(a model.Args) error {
	sim := dump.NewSimulation(dump.DumpSpec{
		Pod:       a.DumpConfig.Pod,
		Namespace: a.DumpConfig.Namespace,
		OutputDir: a.DumpConfig.OutputDir,
	})
	if err := ExecuteOneshot(a, sim); err != nil {
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
			Namespace: a.AdsConfig.Namespace,
			Name:      "adsc",
			IP:        util.GetIP(),
			// TODO: multicluster
			Cluster:  "Kubernetes",
			GrpcOpts: opts,
			Delta:    a.DeltaXDS,
			Labels:   a.Metadata,
		})
	}
	return ExecuteSimulations(a, model.AggregateSimulation{Simulations: sims, Delay: a.AdsConfig.Delay})
}

func Latency(a model.Args) error {
	opts := a.Auth.GrpcOptions("default", "default")

	return ExecuteSimulations(a, &XdsLatencySimulation{
		Namespace: "default",
		Name:      "adsc",
		IP:        util.GetIP(),
		Cluster:   "Kubernetes",
		GrpcOpts:  opts,
	})
}

func ExecuteSimulations(a model.Args, simulation model.Simulation) error {
	ctx, cancel := context.WithCancel(context.Background())
	go captureTermination(ctx, cancel)
	defer cancel()
	go monitoring.StartMonitoring(ctx, 8765)
	simulationContext := model.Context{Context: ctx, Args: a, Client: a.Client, Cancel: cancel}
	if err := simulation.Run(simulationContext); err != nil {
		log.Errorf("failed: %v, starting cleanup", err)
		cleanupErr := simulation.Cleanup(simulationContext)
		return fmt.Errorf("failed to run: %v; cleanup: %v", err, cleanupErr)
	}
	<-ctx.Done()
	return simulation.Cleanup(simulationContext)
}

func ExecuteOneshot(a model.Args, simulation model.Simulation) error {
	ctx, cancel := context.WithCancel(context.Background())
	go captureTermination(ctx, cancel)
	defer cancel()
	go monitoring.StartMonitoring(ctx, 8765)
	simulationContext := model.Context{Context: ctx, Args: a, Client: a.Client, Cancel: cancel}
	if err := simulation.Run(simulationContext); err != nil {
		log.Errorf("failed: %v, starting cleanup", err)
		cleanupErr := simulation.Cleanup(simulationContext)
		return fmt.Errorf("failed to run: %v; cleanup: %v", err, cleanupErr)
	}
	defer log.Infof("simulation completed successfully")
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
