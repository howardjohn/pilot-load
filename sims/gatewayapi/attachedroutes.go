package gatewayapi

import (
	"fmt"
	"strings"
	"time"

	"github.com/howardjohn/pilot-load/pkg/flag"
	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/sims/cluster"
	"github.com/spf13/pflag"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/slices"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gateway "sigs.k8s.io/gateway-api/apis/v1beta1"

	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/test/util/tmpl"
)

type Config struct {
	Gateways    []string
	GracePeriod time.Duration
	Routes      int
}

func Command(f *pflag.FlagSet) flag.Command {
	cfg := Config{
		Routes: 100,
	}

	flag.Register(f, &cfg.Gateways, "gateways", "delay between each connection").Required()
	flag.Register(f, &cfg.Routes, "routes", "number of routes")
	flag.Register(f, &cfg.GracePeriod, "gracePeriod", "delay between each application")
	return flag.Command{
		Name:        "gatewayapi-attachedroutes",
		Description: "apply routes and measure time for attachedRoutes to be valid",
		Details:     "Expected format: `kubectl get vs,gw,dr,sidecar,svc,endpoints,pod,namespace,sa -oyaml -A | kubectl grep`",
		Build: func(args *model.Args) (model.DebuggableSimulation, error) {
			st := map[types.NamespacedName]*Watcher{}
			for _, gw := range cfg.Gateways {
				t := parseNamespacedName(gw)
				st[t] = &Watcher{
					Name:    t,
					Last:    0,
					Samples: nil,
				}
			}
			return &AttachedRoutes{Config: cfg, State: st}, nil
		},
	}
}

func parseNamespacedName(gw string) types.NamespacedName {
	ns, name, _ := strings.Cut(gw, "/")
	return types.NamespacedName{Namespace: ns, Name: name}
}

type ApiDetails struct {
	gvk        schema.GroupVersionKind
	isIstioApi bool
}

type AttachedRoutes struct {
	Config Config
	State  map[types.NamespacedName]*Watcher
}

var _ model.Simulation = &AttachedRoutes{}

const cfgTemplate = `
gracePeriod: {{.GracePeriod}}
namespaces:
- name: mesh
  replicas: 1
  applications:
  - name: app
    replicas: {{.Routes}}
    pods: 0
    type: plain
    configs:
    - name: httproute
      config:
        gateways: {{.Gateways | toJson }}
`

func (i *AttachedRoutes) GetConfig() any {
	return i.Config
}

func (a *AttachedRoutes) Run(ctx model.Context) error {
	gtws := kclient.New[*gateway.Gateway](ctx.Client)
	ctx.Client.RunAndWait(ctx.Done())

	initCh := make(chan struct{})
	processedCh := make(chan struct{})
	teardownProcessedCh := make(chan struct{})
	errCh := make(chan error)

	wasReady := false
	wasProcessed := false

	gtws.AddEventHandler(controllers.ObjectHandler(func(o controllers.Object) {
		gtw := o.(*gateway.Gateway)
		if len(gtw.Status.Listeners) == 0 {
			return
		}
		key := config.NamespacedName(gtw)
		ar := int(gtw.Status.Listeners[0].AttachedRoutes)
		log.Errorf("howardjohn: %v: %v", key, ar)
		cur, f := a.State[key]
		if !f {
			// not watching
			return
		}
		cur.Last = ar
		if cur.Samples == nil {
			cur.Samples = []Sample{}
		}
		if !wasReady {
			//if err := a.AllEqual(0); err != nil {
			//	if !strings.Contains(err.Error(), "not initialized") {
			//		errCh <- fmt.Errorf("initial state invalid: %v", err)
			//		return nil
			//	}
			//	log.Infof("not yet ready: %v", err)
			//	return nil
			//}
			initCh <- struct{}{}
			wasReady = true
			return
		}
		cur.Samples = append(cur.Samples, Sample{
			Time:           time.Now(),
			AttachedRoutes: ar,
		})
		if !wasProcessed {
			if err := a.AllEqual(a.Config.Routes); err != nil {
				log.Infof("not yet processed: %v", err)
				return
			}
			log.Infof("processing done")
			processedCh <- struct{}{}
			wasProcessed = true
			return
		}
		if err := a.AllEqual(0); err != nil {
			log.Infof("not yet torn down: %v", err)
			return
		}
		teardownProcessedCh <- struct{}{}
		return
	}))

	cfg := tmpl.MustEvaluate(cfgTemplate, a.Config)
	clsCfg, err := cluster.ReadConfig(cfg)
	if err != nil {
		return err
	}
	clsSim := cluster.Build(&ctx.Args, clsCfg)
	running := clsSim.Running()
	var routeReady time.Duration
	var teardownstart time.Duration

	clsCtx := ctx.WithCancel()
	var handle simulation.Running
	var startTime time.Time
	for {
		select {
		case <-running:
			routeReady = time.Since(startTime)
			running = nil
		case <-ctx.Done():
			clsCtx.Cancel()
			handle.Wait()
			return nil
		case <-initCh:
			// Start the routes
			startTime = time.Now()
			handle = simulation.RunSimulation(clsCtx, clsSim)
		case err := <-errCh:
			return err
		case <-processedCh:
			// We are done, close things out
			clsCtx.Cancel()
			teardownstart = time.Since(startTime)
		case <-teardownProcessedCh:
			if err := handle.Wait(); err != nil {
				return err
			}
			a.Report(startTime, routeReady, teardownstart)
			ctx.Cancel()
			return nil
		}
	}
}

func (a *AttachedRoutes) Cleanup(ctx model.Context) error {
	return nil
}

func (i *AttachedRoutes) AllEqual(want int) error {
	for _, w := range i.State {
		if w.Samples == nil {
			return fmt.Errorf("%v not initialized", w.Name)
		}
		if w.Last != want {
			return fmt.Errorf("want %d, got %d for %v", want, w.Last, w.Name)
		}
	}
	return nil
}

func (i *AttachedRoutes) Report(t0 time.Time, ready time.Duration, teardownstart time.Duration) {

	log.WithLabels("ready time", ready, "teardown time", teardownstart).Infof("Test complete")
	for name, w := range i.State {
		top := slices.IndexFunc(w.Samples, func(x Sample) bool {
			return x.AttachedRoutes == i.Config.Routes
		})
		last := w.Samples[len(w.Samples)-1]

		topT := w.Samples[top].Time.Sub(t0) - ready
		bottomT := last.Time.Sub(t0) - teardownstart
		log.WithLabels("name", name, "add-all", topT, "remove-all", bottomT, "writes", len(w.Samples)).Infof("complete")
	}
}

type Watcher struct {
	Name    types.NamespacedName
	Last    int
	Samples []Sample
}

type Sample struct {
	Time           time.Time
	AttachedRoutes int
}