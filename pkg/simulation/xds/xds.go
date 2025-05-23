package xds

import (
	"context"

	"google.golang.org/grpc"

	"github.com/howardjohn/pilot-load/adsc"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type Simulation struct {
	Labels         map[string]string
	Metadata       map[string]string
	Namespace      string
	ServiceAccount string
	Name           string
	IP             string
	// Defaults to "Kubernetes"
	Cluster string
	AppType model.AppType

	GrpcOpts []grpc.DialOption

	cancel context.CancelFunc
	done   chan struct{}
	Delta  bool
}

func clone(m map[string]string) map[string]interface{} {
	n := map[string]interface{}{}
	for k, v := range m {
		n[k] = v
	}
	return n
}

func (x *Simulation) Run(ctx model.Context) error {
	c, cancel := context.WithCancel(ctx.Context)
	x.cancel = cancel
	x.done = make(chan struct{})
	cluster := x.Cluster
	if cluster == "" {
		cluster = "Kubernetes"
	}
	meta := clone(ctx.Args.Metadata)
	meta["ISTIO_VERSION"] = "1.24.0-pilot-load"
	meta["CLUSTER_ID"] = cluster
	meta["LABELS"] = x.Labels
	meta["NAMESPACE"] = x.Namespace
	meta["SERVICE_ACCOUNT"] = x.ServiceAccount
	meta["PROXY_CONFIG"] = map[string]string{}
	for k, v := range x.Metadata {
		meta[k] = v
	}
	go func() {
		nt := string(x.AppType)
		if nt == "gateway" {
			nt = "router"
		}
		adsc.Connect(ctx.Args.PilotAddress, &adsc.Config{
			Namespace: x.Namespace,
			Workload:  x.Name + "-" + x.IP,
			Meta:      meta,
			NodeType:  nt,
			IP:        x.IP,
			Context:   c,
			GrpcOpts:  x.GrpcOpts,
			Delta:     x.Delta,
		})
		close(x.done)
	}()
	return nil
}

func (x *Simulation) Cleanup(ctx model.Context) error {
	if x == nil {
		return nil
	}
	if x.cancel != nil {
		x.cancel()
	}
	<-x.done
	return nil
}

var _ model.Simulation = &Simulation{}
