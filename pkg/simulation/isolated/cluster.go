package isolated

import (
	"net"
	"net/http"
	"time"

	xdstest "istio.io/istio/pilot/test/xds"
	kubelib "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/test"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/howardjohn/pilot-load/pkg/simulation/cluster"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type IsolatedSpec struct {
	Config         model.ClusterConfig
	Fake           kubelib.Client
	Listener       net.Listener
	MetricsHandler http.Handler
}

type Isolated struct {
	Spec          *IsolatedSpec
	Cluster       *cluster.Cluster
	FakeDiscovery *FakeDiscovery
}

var _ model.Simulation = &Isolated{}

func NewCluster(s IsolatedSpec) *Isolated {
	fd := &FakeDiscovery{
		Fake:           s.Fake,
		Listener:       s.Listener,
		MetricsHandler: s.MetricsHandler,
		Ready:          make(chan struct{}),
	}
	is := &Isolated{Spec: &s, FakeDiscovery: fd}
	c := cluster.NewCluster(cluster.ClusterSpec{
		Config: s.Config,
	})
	is.Cluster = c

	return is
}

func (c *Isolated) Run(ctx model.Context) error {
	errCh := make(chan error, 2)
	go func() {
		if err := c.Cluster.Run(ctx); err != nil {
			errCh <- err
		}
	}()
	select {
	case <-c.Cluster.Running():
		log.Infof("fake configuration setup, launching Istiod")
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return nil
	}
	go func() {
		if err := c.FakeDiscovery.Run(ctx); err != nil {
			errCh <- err
		}
	}()
	running := c.FakeDiscovery.Running()
	select {
	case err := <-errCh:
		log.Infof("got error: %v", err)
		return err
	case <-running:
		log.Infof("running complete")
		return nil
	case <-ctx.Done():
		log.Infof("ctx complete")
		return nil
	}
}

func (c *Isolated) Cleanup(ctx model.Context) error {
	return c.Cluster.Cleanup(ctx)
}

type FakeDiscovery struct {
	Fake           kubelib.Client
	Listener       net.Listener
	MetricsHandler http.Handler
	Ready          chan struct{}
}

var _ model.Simulation = &FakeDiscovery{}

func (f *FakeDiscovery) Run(ctx model.Context) error {
	t0 := time.Now()
	done := make(chan struct{})
	defer func() {
		close(done)
	}()
	go test.Wrap(func(t test.Failer) {
		ds := xdstest.NewFakeDiscoveryServer(t, xdstest.FakeOptions{
			DebounceTime: time.Millisecond * 50,
			KubeClientBuilder: func(objects ...runtime.Object) kubelib.Client {
				return f.Fake
			},
			ListenerBuilder: func() (net.Listener, error) {
				return f.Listener, nil
			},
		})
		ds.Discovery.InitDebug(f.MetricsHandler.(*http.ServeMux), false, func() map[string]string {
			return nil
		})
		close(f.Ready)
		log.Infof("Istiod is ready (%v)", time.Since(t0))
		<-ctx.Done()
	})

	// run forever, until we are canceled
	<-ctx.Done()
	return nil
}

func (f *FakeDiscovery) Cleanup(ctx model.Context) error {
	return nil
}

func (f *FakeDiscovery) Running() chan struct{} {
	return f.Ready
}
