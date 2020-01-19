package simulation

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"
)

type Args struct {
	PilotAddress string
}

func Simple(a Args) error {
	wl := NewWorkload(WorkloadSpec{
		App:            "app",
		Node:           "node",
		Namespace:      "workload",
		ServiceAccount: "default",
		Instances:      10,
		Scaling:        time.Second * 5,
	})
	if err := ExecuteSimulations(a, wl); err != nil {
		return fmt.Errorf("error executing: %v", err)
	}
	return nil
}

func ExecuteSimulations(a Args, simulation Simulation) error {
	ctx, cancel := context.WithCancel(context.Background())
	go captureTermination(ctx, cancel)
	return simulation.Run(Context{ctx, a})
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
