package app

import (
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/maps"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type ApplicationSpec struct {
	App                 string
	Node                func() string
	Namespace           string
	ServiceAccount      string
	Instances           int
	Type                model.AppType
	TemplateDefinitions model.TemplateDefinitions
	Templates           []model.ConfigTemplate
	Labels              map[string]string
}

type Application struct {
	Spec          *ApplicationSpec
	pods          []*Pod
	service       *Service
	kgateways     []*config.KubeGateway
	workloadEntry *config.WorkloadEntry
	workloadGroup *config.WorkloadGroup
	serviceEntry  *config.ServiceEntry
	configs       []*config.Templated
}

var (
	_ model.Simulation            = &Application{}
	_ model.ScalableSimulation    = &Application{}
	_ model.RefreshableSimulation = &Application{}
)

func NewApplication(s ApplicationSpec) *Application {
	w := &Application{Spec: &s}

	// Apply common CRDs to all app types
	for _, tmpl := range s.Templates {
		cfg := maps.Clone(tmpl.Config)
		if cfg == nil {
			cfg = map[string]any{}
		}
		if _, f := cfg[config.Namespace]; !f {
			cfg[config.Namespace] = s.Namespace
		}
		if _, f := cfg[config.Name]; !f {
			cfg[config.Name] = s.App
		}
		w.configs = append(w.configs, config.NewTemplated(config.TemplatedSpec{
			Template: s.TemplateDefinitions.Get(tmpl.Name),
			Config:   cfg,
			Refresh:  tmpl.Refresh,
		}))
	}

	// Apply CRDs for External app type and return
	if s.Type == model.ExternalType {
		w.serviceEntry = config.NewServiceEntry(config.ServiceEntrySpec{
			App:       s.App,
			Namespace: s.Namespace,
			AppType:   s.Type,
		})
		return w
	}

	// Apply CRDs for VM app type and return
	if s.Type == model.VMType {
		w.serviceEntry = config.NewServiceEntry(config.ServiceEntrySpec{
			App:       s.App,
			Namespace: s.Namespace,
			AppType:   s.Type,
		})

		w.workloadGroup = config.NewWorkloadGroup(config.WorkloadGroupSpec{
			App:       s.App,
			Namespace: s.Namespace,
		})

		w.workloadEntry = config.NewWorkloadEntry(config.WorkloadEntrySpec{
			App:       s.App,
			Namespace: s.Namespace,
		})
		return w
	}

	// Currently we never use Deployment since its pretty slow - create Pods manually instead
	for i := 0; i < s.Instances; i++ {
		w.pods = append(w.pods, w.makePod())
	}

	w.service = NewService(ServiceSpec{
		App:       s.App,
		Namespace: s.Namespace,
		Labels:    s.Labels,
		Waypoint:  s.Type == model.WaypointType,
	})

	if s.Type == model.WaypointType {
		gw := config.NewKubeGateway(config.KubeGatewaySpec{
			App:       s.App,
			Namespace: s.Namespace,
			Waypoint:  true,
		})
		w.kgateways = append(w.kgateways, gw)
	}

	return w
}

func (w *Application) GetConfigs() []model.RefreshableSimulation {
	sims := []model.RefreshableSimulation{}
	if w.workloadEntry != nil {
		sims = append(sims, w.workloadEntry)
	}
	for _, cfg := range w.configs {
		sims = append(sims, cfg)
	}

	return sims
}

func (w *Application) makePod() *Pod {
	s := w.Spec
	return NewPod(PodSpec{
		ServiceAccount: s.ServiceAccount,
		Node:           s.Node(),
		App:            s.App,
		Namespace:      s.Namespace,
		AppType:        s.Type,
	})
}

func (w *Application) getSims() []model.Simulation {
	sims := []model.Simulation{}

	for _, cfg := range w.configs {
		sims = append(sims, cfg)
	}
	if w.service != nil {
		sims = append(sims, w.service)
	}
	if w.serviceEntry != nil {
		sims = append(sims, w.serviceEntry)
	}
	if w.workloadEntry != nil {
		sims = append(sims, w.workloadEntry)
	}
	if w.workloadGroup != nil {
		sims = append(sims, w.workloadGroup)
	}
	for _, gw := range w.kgateways {
		sims = append(sims, gw)
	}
	for _, p := range w.pods {
		sims = append(sims, p)
	}
	return sims
}

func (w *Application) Run(ctx model.Context) (err error) {
	return model.AggregateSimulation{Simulations: w.getSims()}.RunParallel(ctx)
}

func (w *Application) Cleanup(ctx model.Context) error {
	return model.AggregateSimulation{Simulations: model.ReverseSimulations(w.getSims())}.CleanupParallel(ctx)
}

func (w *Application) Refresh(ctx model.Context) (string, error) {
	// TODO: implement for Deployment
	if len(w.pods) == 0 {
		return "skipped, no pods", nil
	}

	i := 0
	if len(w.pods) > 1 {
		i = rand.IntnRange(0, len(w.pods)-1)
	}

	newPod := w.makePod()
	removed := w.pods[i]

	w.pods[i] = newPod
	if err := newPod.Run(ctx); err != nil {
		return "", err
	}

	if err := removed.Cleanup(ctx); err != nil {
		return "", err
	}

	return newPod.Spec.Namespace + "/" + newPod.Name(), nil
}

func (w *Application) Scale(ctx model.Context, delta int) error {
	return w.ScaleTo(ctx, len(w.pods)+delta)
}

func (w *Application) ScaleTo(ctx model.Context, n int) error {
	log.Infof("%v: scaling pod from %d -> %d", w.Spec.App, len(w.pods), n)
	for n < len(w.pods) && n >= 0 {
		i := 0
		if len(w.pods) > 1 {
			i = rand.IntnRange(0, len(w.pods)-1)
		}
		// Remove the element at index i from a.
		old := w.pods[i]
		w.pods[i] = w.pods[len(w.pods)-1] // Copy last element to index i.
		w.pods[len(w.pods)-1] = nil       // Erase last element (write zero value).
		w.pods = w.pods[:len(w.pods)-1]   // Truncate slice.
		if err := old.Cleanup(ctx); err != nil {
			log.Infof("err: %v", err)
			return err
		}
	}

	for n > len(w.pods) {
		pod := w.makePod()
		w.pods = append(w.pods, pod)
		if err := pod.Run(ctx); err != nil {
			return err
		}
	}

	return nil
}
