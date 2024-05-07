package isolated

import (
	"github.com/howardjohn/pilot-load/pkg/simulation/cluster"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	xdstest "istio.io/istio/pilot/test/xds"
	kubelib "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/test"
	"k8s.io/apimachinery/pkg/runtime"
	"net"
	"net/http"
	"time"
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
		errCh <- c.Cluster.Run(ctx)
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
		errCh <- c.FakeDiscovery.Run(ctx)
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
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
