package model

import (
	"context"

	"github.com/howardjohn/pilot-load/pkg/kube"
)

type Simulation interface {
	Run(ctx Context) error
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
