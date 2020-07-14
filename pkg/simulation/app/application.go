package app

import (
	"fmt"

	"istio.io/pkg/log"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type ApplicationSpec struct {
	App            string
	Node           string
	Namespace      string
	ServiceAccount string
	Instances      int
	PodType        model.PodType
	GatewayConfig  model.GatewayConfig
}

type Application struct {
	Spec           *ApplicationSpec
	endpoint       *Endpoint
	pods           []*Pod
	service        *Service
	virtualService *config.VirtualService
	gateway        *config.Gateway
	destRule       *config.DestinationRule
}

var _ model.Simulation = &Application{}
var _ model.ScalableSimulation = &Application{}
var _ model.RefreshableSimulation = &Application{}

func NewApplication(s ApplicationSpec) *Application {
	w := &Application{Spec: &s}

	for i := 0; i < s.Instances; i++ {
		w.pods = append(w.pods, w.makePod())
	}

	w.endpoint = NewEndpoint(EndpointSpec{
		Node:      s.Node,
		App:       s.App,
		Namespace: s.Namespace,
		IPs:       w.getIps(),
	})
	w.service = NewService(ServiceSpec{
		App:       s.App,
		Namespace: s.Namespace,
	})
	if s.GatewayConfig.Enabled {
		w.gateway = config.NewGateway(config.GatewaySpec{
			Name:      s.GatewayConfig.Name,
			App:       s.App,
			Namespace: s.Namespace,
		})
	}
	if s.PodType != model.ExternalType {
		w.virtualService = config.NewVirtualService(config.VirtualServiceSpec{
			App:       s.App,
			Namespace: s.Namespace,
			Gateways:  s.GatewayConfig.VirtualServices,
			Subsets:   []config.SubsetSpec{{"a", 50}, {"b", 50}},
		})
		w.destRule = config.NewDestinationRule(config.DestinationRuleSpec{
			App:       s.App,
			Namespace: s.Namespace,
			Subsets:   []string{"a", "b"},
		})
	}
	return w
}

func (w *Application) GetConfigs() []model.RefreshableSimulation {
	return []model.RefreshableSimulation{w.virtualService}
}

func (w *Application) makePod() *Pod {
	s := w.Spec
	return NewPod(PodSpec{
		ServiceAccount: s.ServiceAccount,
		Node:           s.Node,
		App:            s.App,
		Namespace:      s.Namespace,
		PodType:        s.PodType,
	})
}

func (w *Application) getSims() []model.Simulation {
	sims := []model.Simulation{w.service}
	if w.virtualService != nil {
		sims = append(sims, w.virtualService)
	}
	if w.destRule != nil {
		sims = append(sims, w.destRule)
	}
	if w.gateway != nil {
		sims = append(sims, w.gateway)
	}
	for _, p := range w.pods {
		sims = append(sims, p)
	}
	sims = append(sims, w.endpoint)
	return sims
}

func (w *Application) Run(ctx model.Context) (err error) {
	return model.AggregateSimulation{w.getSims()}.RunParallel(ctx)
}

func (w *Application) Cleanup(ctx model.Context) error {
	return model.AggregateSimulation{model.ReverseSimulations(w.getSims())}.CleanupParallel(ctx)
}

// TODO scale up first, but make sure we don't immediately scale that one down
func (w *Application) Refresh(ctx model.Context) error {
	if err := w.Scale(ctx, -1); err != nil {
		return fmt.Errorf("scale down: %v", err)
	}
	if err := w.Scale(ctx, 1); err != nil {
		return fmt.Errorf("scale up: %v", err)
	}
	return nil
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

	if err := w.endpoint.SetAddresses(ctx, w.getIps()); err != nil {
		return fmt.Errorf("endpoints: %v", err)
	}
	return nil
}

func (w Application) getIps() map[string]string {
	ret := map[string]string{}
	for _, p := range w.pods {
		ret[p.Name()] = p.Spec.IP
	}
	return ret
}
