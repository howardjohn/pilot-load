package xds

import (
	"context"
	"crypto/tls"
	"strings"

	"github.com/howardjohn/pilot-load/adsc"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type Simulation struct {
	Labels    map[string]string
	Namespace string
	Name      string
	IP        string
	// Defaults to "Kubernetes"
	Cluster string
	PodType model.PodType

	// Certificate options. If not provided, will use plaintext
	RootCert   []byte
	ClientCert tls.Certificate

	cancel context.CancelFunc
	done   chan struct{}
}

func (x *Simulation) Run(ctx model.Context) error {
	c, cancel := context.WithCancel(ctx.Context)
	x.cancel = cancel
	x.done = make(chan struct{})
	cluster := x.Cluster
	if cluster == "" {
		cluster = "Kubernetes"
	}
	meta := map[string]interface{}{
		"ISTIO_VERSION": "1.7.0",
		"CLUSTER_ID":    cluster,
		"LABELS":        x.Labels,
		"NAMESPACE":     x.Namespace,
		"SDS":           "true",
	}
	go func() {
		adsc.Connect(ctx.Args.PilotAddress, &adsc.Config{
			Namespace: x.Namespace,
			Workload:  x.Name,
			Meta:      meta,
			NodeType:  string(x.PodType),
			IP:        x.IP,
			Context:   c,

			SystemCerts: strings.HasSuffix(ctx.Args.PilotAddress, ":443"),
			RootCert:    x.RootCert,
			ClientCert:  x.ClientCert,
		})
		close(x.done)
	}()
	return nil
}

func (x *Simulation) Cleanup(ctx model.Context) error {
	if x == nil {
		return nil
	}
	x.cancel()
	<-x.done
	return nil
}

var _ model.Simulation = &Simulation{}
