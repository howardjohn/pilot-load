package xds

import (
	"context"

	"github.com/howardjohn/pilot-load/adsc"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type XdsSimulation struct {
	Labels    map[string]string
	Namespace string
	Name      string
	IP        string
	// Defaults to "Kubernetes"
	Cluster string
	cancel  context.CancelFunc
	done    chan struct{}
}

func (x *XdsSimulation) Run(ctx model.Context) error {
	c, cancel := context.WithCancel(context.Background())
	x.cancel = cancel
	x.done = make(chan struct{})
	cluster := x.Cluster
	if cluster == "" {
		cluster = "Kubernetes"
	}
	meta := map[string]interface{}{
		"ISTIO_VERSION": "1.6.0",
		"CLUSTER_ID":    cluster,
		"LABELS":        x.Labels,
		"NAMESPACE":     x.Namespace,
	}
	go func() {
		// TODO trigger full injection and CA bootstrap flow
		// TODO use XDS v3
		// TODO allow routers
		adsc.Connect(c, ctx.Args.PilotAddress, &adsc.Config{
			Namespace: x.Namespace,
			Workload:  x.Name,
			Meta:      meta,
			NodeType:  "sidecar",
			IP:        x.IP,
			Verbose:   false,
		})
		close(x.done)
	}()
	return nil
}

func (x XdsSimulation) Cleanup(ctx model.Context) error {
	x.cancel()
	<-x.done
	return nil
}

var _ model.Simulation = &XdsSimulation{}
