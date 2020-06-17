package xds

import (
	"github.com/howardjohn/pilot-load/adsc"
	"github.com/howardjohn/pilot-load/client"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type XdsSimulation struct {
	Labels    map[string]string
	Namespace string
	Name      string
	IP        string
	// Defaults to "Kubernetes"
	Cluster string
}

func (x XdsSimulation) Run(ctx model.Context) error {
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
		client.Connect(ctx, ctx.Args.PilotAddress, &adsc.Config{
			Namespace: x.Namespace,
			Workload:  x.Name,
			Meta:      meta,
			NodeType:  "sidecar",
			IP:        x.IP,
			Verbose:   false,
		})
	}()
	return nil
}

func (x XdsSimulation) Cleanup(ctx model.Context) error {
	return nil
}

var _ model.Simulation = &XdsSimulation{}
