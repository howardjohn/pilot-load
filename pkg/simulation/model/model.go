package model

import (
	"context"

	"github.com/howardjohn/pilot-load/pkg/kube"
)

type Simulation interface {
	// Run starts the simulation. If the simulation is long lived, this should be done asynchronously,
	// watching ctx.Done() for termination.
	Run(ctx Context) error
	// Cleanup tears down the simulation.
	Cleanup(ctx Context) error
}

type Args struct {
	PilotAddress string
	NodeMetadata string
	KubeConfig   string
}

type Context struct {
	context.Context
	Args   Args
	Client *kube.Client
}
