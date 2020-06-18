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

func makeTicker(t time.Duration) <-chan time.Time {
	if t <= 0 {
		// Fake timer
		return make(chan time.Time)
	}
	return time.NewTicker(t).C
}

func (s *ClusterScaler) Run(ctx model.Context) error {
	c, cancel := context.WithCancel(ctx.Context)
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		nsT := makeTicker(s.Cluster.Spec.Scaler.NamespacesDelay)
		svcT := makeTicker(s.Cluster.Spec.Scaler.ServicesDelay)
		instanceT := makeTicker(s.Cluster.Spec.Scaler.InstancesDelay)
		for {
			// TODO: more customization around everything here
			select {
			case <-c.Done():
				return
			case <-nsT:
				log.Errorf("scaling namespace not implemented")
			case <-svcT:
				for _, ns := range s.Cluster.namespaces {
					if err := ns.InsertService(ctx, model.ServiceArgs{Instances: 1}); err != nil {
						log.Errorf("failed to scale namespace: %v", err)
					}
				}
			case <-instanceT:
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
