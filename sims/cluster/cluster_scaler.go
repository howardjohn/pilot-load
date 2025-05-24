package cluster

import (
	"context"
	"math/rand"
	"time"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/lthibault/jitterbug"

	"istio.io/istio/pkg/log"
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
	tck := jitterbug.New(
		t,
		&jitterbug.Norm{Stdev: t / 5},
	)
	return tck.C
}

func (s *ClusterScaler) Run(ctx model.Context) error {
	c, cancel := context.WithCancel(ctx.Context)
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		instanceJitterT := makeTicker(time.Duration(s.Cluster.Spec.Config.Jitter.Workloads))
		configJitterT := makeTicker(time.Duration(s.Cluster.Spec.Config.Jitter.Config))
		for {
			// TODO: more customization around everything here
			select {
			case <-c.Done():
				return
			case <-instanceJitterT:
				wls := s.Cluster.GetRefreshableInstances()
				if len(wls) == 0 {
					log.Warnf("no instances to scale")
					continue
				}
				wl := wls[rand.Intn(len(wls))]
				if info, err := wl.Refresh(ctx); err != nil {
					log.Errorf("failed to jitter workloads: %v", err)
				} else {
					log.Infof("refreshed workload %s (%T)", info, wl)
				}
			case <-configJitterT:
				cfgs := s.Cluster.GetRefreshableConfig()
				if len(cfgs) == 0 {
					log.Warnf("no configs to scale")
					continue
				}
				cfg := cfgs[rand.Intn(len(cfgs))]
				if info, err := cfg.Refresh(ctx); err != nil {
					log.Errorf("failed to jitter configs: %v", err)
				} else {
					log.Infof("refreshed config %s (%T)", info, cfg)
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
