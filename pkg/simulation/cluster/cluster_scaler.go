package cluster

import (
	"context"
	"time"

	"istio.io/pkg/log"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type ClusterScaler struct {
	Cluster *Cluster
	cancel  context.CancelFunc
	done    chan struct{}
}

func (s *ClusterScaler) Run(ctx model.Context) error {
	c, cancel := context.WithCancel(ctx.Context)
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		for {
			select {
			case <-c.Done():
				return
				// Every 15s, scale up all workloads by 1
			case <-time.After(time.Second * 3):
				for _, ns := range s.Cluster.namespaces {
					for _, w := range ns.workloads {
						if err := w.Scale(ctx, 1); err != nil {
							log.Errorf("failed to scale workload: %v", err)
						}
					}
				}
			}
		}
	}()
	return nil
}

func (s *ClusterScaler) Cleanup(ctx model.Context) error {
	if s == nil {
		return nil
	}
	s.cancel()
	<-s.done
	return nil
}

var _ model.Simulation = &ClusterScaler{}
