package simulation

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"golang.org/x/sync/errgroup"
)

type Args struct {
	PilotAddress string
}

func Simple(a Args) error {
	wl := Workload{
		App:            "app",
		Node:           "node",
		Namespace:      "workload",
		ServiceAccount: "default",
		Instances:      3,
	}
	run, err := wl.Run(a)
	if err != nil {
		panic(err)
	}
	if err := ExecuteSimulations([]Runner{run}); err != nil {
		return fmt.Errorf("error executing: %v", err)
	}
	return nil
}

func ExecuteSimulations(runners []Runner) error {
	g, c := errgroup.WithContext(context.Background())
	ctx, cancel := context.WithCancel(c)
	go captureTermination(ctx, cancel)
	for _, r := range runners {
		g.Go(func() error {
			return r(ctx)
		})
	}

	return g.Wait()
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
