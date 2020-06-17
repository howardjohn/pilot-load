package simulation

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/app"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"
)

func Simple(a model.Args) error {
	numWorkloads := 1
	ns := app.NewNamespace(app.NamespaceSpec{
		Name: "workload",
	})
	sa := app.NewServiceAccount(app.ServiceAccountSpec{
		Namespace: ns.Spec.Name,
		Name:      "default",
	})
	w := app.NewWorkload(app.WorkloadSpec{
		App:            fmt.Sprintf("app-%d", numWorkloads),
		Node:           "node",
		Namespace:      ns.Spec.Name,
		ServiceAccount: sa.Spec.Name,
		Instances:      2,
	})

	sim := model.AggregateSimulation{[]model.Simulation{ns, sa, w}}
	if err := ExecuteSimulations(a, sim); err != nil {
		return fmt.Errorf("error executing: %v", err)
	}
	return nil
}

func Adsc(a model.Args) error {
	return ExecuteSimulations(a, xds.XdsSimulation{
		Namespace: "default",
		Name:      "adsc",
		IP:        "1.2.3.4",
		// TODO: multicluster
		Cluster: "pilot-load",
	})
}

func ExecuteSimulations(a model.Args, simulation model.Simulation) error {
	cl, err := kube.NewClient(a.KubeConfig)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	go captureTermination(ctx, cancel)
	defer cancel()
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
	defer func() {
		signal.Stop(c)
	}()
	select {
	case <-c:
		cancel()
	case <-ctx.Done():
	}
}
