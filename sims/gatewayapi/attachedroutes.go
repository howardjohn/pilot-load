package gatewayapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Gateways     []string
	GracePeriod  time.Duration
	VictoriaLogs string
	Routes       int
}

func Command(f *pflag.FlagSet) flag.Command {
	cfg := Config{
		Routes: 100,
	}

	flag.Register(f, &cfg.Gateways, "gateways", "delay between each connection").Required()
	flag.Register(f, &cfg.Routes, "routes", "number of routes")
	flag.Register(f, &cfg.VictoriaLogs, "victoria", "victoria-logs address")
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

	startTime time.Time
	ready time.Duration
	teardownStart time.Duration
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

func (a *AttachedRoutes) GetConfig() any {
	return a.Config
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
			if err := a.AllEqual(key, 0); err != nil {
				if !strings.Contains(err.Error(), "not initialized") {
					errCh <- fmt.Errorf("initial state invalid: %v", err)
					return
				}
				log.Infof("not yet ready: %v", err)
				return
			}
			initCh <- struct{}{}
			wasReady = true
			return
		}
		cur.Samples = append(cur.Samples, Sample{
			Time:           time.Now(),
			AttachedRoutes: ar,
		})
		if !wasProcessed {
			if err := a.AllEqual(key, a.Config.Routes); err != nil {
				log.Infof("not yet processed: %v", err)
				return
			}
			log.Infof("processing done")
			processedCh <- struct{}{}
			wasProcessed = true
			return
		}
		if err := a.AllEqual(key, 0); err != nil {
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

	clsCtx := ctx.WithCancel()
	var handle simulation.Running
	for {
		select {
		case <-running:
			a.ready = time.Since(a.startTime)
			running = nil
		case <-ctx.Done():
			clsCtx.Cancel()
			handle.Wait()
			return nil
		case <-initCh:
			// Start the routes
			a.startTime = time.Now()
			handle = simulation.RunSimulation(clsCtx, clsSim)
		case err := <-errCh:
			return err
		case <-processedCh:
			// We are done, close things out
			clsCtx.Cancel()
			a.teardownStart = time.Since(a.startTime)
		case <-teardownProcessedCh:
			if err := handle.Wait(); err != nil {
				return err
			}

			ctx.Cancel()
			return nil
		}
	}
}

func (a *AttachedRoutes) Cleanup(ctx model.Context) error {
	a.Report()
	return nil
}

func (a *AttachedRoutes) AllEqual(key types.NamespacedName, want int) error {
	processed := func(w *Watcher) error {
		if w.Samples == nil {
			return fmt.Errorf("%v not initialized", w.Name)
		}
		if w.Last != want {
			return fmt.Errorf("want %d, got %d for %v", want, w.Last, w.Name)
		}
		return nil
	}
	// Check the one we are currently processing to avoid confusing logs
	if err := processed(a.State[key]); err != nil {
		return err
	}
	for _, w := range a.State {
		if err := processed(w); err != nil {
			return err
		}
	}
	return nil
}

func (a *AttachedRoutes) Report() {

	log.WithLabels("ready time", a.ready, "teardown time", a.teardownStart).Infof("Test complete")
	for name, w := range a.State {
		top := slices.IndexFunc(w.Samples, func(x Sample) bool {
			return x.AttachedRoutes == a.Config.Routes
		})
		if top == -1 {
			log.WithLabels("name", name).Errorf("failed to complete test")
			continue
		}
		last := w.Samples[len(w.Samples)-1]

		topT := w.Samples[top].Time.Sub(a.startTime) - a.ready
		bottomT := last.Time.Sub(a.startTime) - a.teardownStart
		log.WithLabels("name", name, "add-all", topT, "remove-all", bottomT, "writes", len(w.Samples)).Infof("complete")
	}

	if a.Config.VictoriaLogs != "" {
		var entries []VicLogEntry
		for name, w := range a.State {
			for _, sample := range w.Samples {
				entries = append(entries, VicLogEntry{
					Message: "event",
					Gateway: name.String(),
					Time:    sample.Time.UnixNano(),
					Value:   sample.AttachedRoutes,
				})
			}
		}
		r, w := io.Pipe()
		go func() {
			enc := json.NewEncoder(w)
			for _, item := range entries {
				enc.Encode(item)
			}
			w.Close()
		}()
		resp, err := http.DefaultClient.Post(a.Config.VictoriaLogs+"/insert/jsonline?_stream_fields=gateway", "application/stream+json", r)
		if err != nil {
			log.Errorf("error posting victoria logs: %v", err)
		} else {
			log.Infof("victoria logs: %s", resp.Status)
		}
	}
}

type VicLogEntry struct {
	Message string `json:"_msg"`
	Gateway string `json:"gateway"`
	Time    int64  `json:"_time"`
	Value   int    `json:"value"`
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