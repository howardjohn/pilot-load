package app

import (
	"fmt"

	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"istio.io/pkg/log"
	"k8s.io/apimachinery/pkg/util/rand"
)

type ApplicationSpec struct {
	App            string
	Node           string
	Namespace      string
	ServiceAccount string
	Instances      int
	Type           model.AppType
	GatewayConfig  model.GatewayConfig
	Istio          model.IstioApplicationConfig
	Labels         map[string]string
	ClusterType    model.ClusterType
}

type Application struct {
	Spec           *ApplicationSpec
	endpoint       *Endpoint
	pods           []*Pod
	deployment     *Deployment
	service        *Service
	virtualService *config.VirtualService
	gateways       []*config.Gateway
	secrets        []*config.Secret
	destRule       *config.DestinationRule
	workloadEntry  *config.WorkloadEntry
	workloadGroup  *config.WorkloadGroup
	serviceEntry   *config.ServiceEntry
	envoyFilter    *config.EnvoyFilter
	sidecar        *config.Sidecar
}

var (
	_ model.Simulation            = &Application{}
	_ model.ScalableSimulation    = &Application{}
	_ model.RefreshableSimulation = &Application{}
)

func NewApplication(s ApplicationSpec) *Application {
	w := &Application{Spec: &s}

	// Apply common CRDs to all app types
	if s.Istio.Default == true || s.Istio.VirtualService != nil {
		var gateways []string
		if s.Istio.VirtualService != nil && len(s.Istio.VirtualService.Gateways) != 0 {
			gateways = s.Istio.VirtualService.Gateways
		}
		w.virtualService = config.NewVirtualService(config.VirtualServiceSpec{
			App:       s.App,
			Namespace: s.Namespace,
			Gateways:  gateways,
			Subsets:   []config.SubsetSpec{{Name: "a", Weight: 100}},
		})
	}
	if s.Istio.Default == true || s.Istio.DestinationRule != nil {
		w.destRule = config.NewDestinationRule(config.DestinationRuleSpec{
			App:       s.App,
			Namespace: s.Namespace,
			Subsets:   []string{"a"},
		})
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

	//Apply CRDs for sidecar and GW app type
	if s.Istio.Default == true || s.Istio.EnvoyFilter != nil {
		w.envoyFilter = config.NewEnvoyFilter(config.EnvoyFilterSpec{
			App:       s.App,
			Namespace: s.Namespace,
			APIScope:  model.Application,
		})
	}
	if s.Istio.Default == true || s.Istio.Sidecar != nil {
		w.sidecar = config.NewSidecar(config.SidecarSpec{
			App:       s.App,
			Namespace: s.Namespace,
			APIScope:  model.Application,
		})
	}

	// Currently we never use Deployment since its pretty slow
	//	w.deployment = NewDeployment(DeploymentSpec{
	//		ServiceAccount: s.ServiceAccount,
	//		Replicas:       s.Instances,
	//		Node:           s.Node,
	//		App:            s.App,
	//		Namespace:      s.Namespace,
	//		type:           s.type,
	//		ClusterType:    s.ClusterType,
	//	})
	for i := 0; i < s.Instances; i++ {
		w.pods = append(w.pods, w.makePod())
	}

	if s.ClusterType != model.FakeNode {
		w.endpoint = NewEndpoint(EndpointSpec{
			Node:        s.Node,
			App:         s.App,
			Namespace:   s.Namespace,
			IPs:         w.getIps(),
			ClusterType: s.ClusterType,
		})
	}
	w.service = NewService(ServiceSpec{
		App:         s.App,
		Namespace:   s.Namespace,
		ClusterType: s.ClusterType,
	})

	if s.Type == model.GatewayType {
		for i := 0; i < s.GatewayConfig.Replicas; i++ {
			gw := config.NewGateway(config.GatewaySpec{
				Name:      s.GatewayConfig.Name,
				App:       s.App,
				Namespace: s.Namespace,
			})
			w.gateways = append(w.gateways, gw)
			w.secrets = append(w.secrets, config.NewSecret(config.SecretSpec{
				Namespace: s.Namespace,
				Name:      gw.Name(),
			}))
		}
	}

	return w
}

func (w *Application) GetConfigs() []model.RefreshableSimulation {
	sims := []model.RefreshableSimulation{}
	if w.virtualService != nil {
		sims = append(sims, w.virtualService)
	}
	if w.destRule != nil {
		sims = append(sims, w.destRule)
	}
	if w.envoyFilter != nil {
		sims = append(sims, w.envoyFilter)
	}
	if w.sidecar != nil {
		sims = append(sims, w.sidecar)
	}
	if w.workloadEntry != nil {
		sims = append(sims, w.workloadEntry)
	}

	return sims
}

func (w *Application) GetSecrets() []model.RefreshableSimulation {
	sims := []model.RefreshableSimulation{}
	for _, scr := range w.secrets {
		sims = append(sims, scr)
	}
	return sims
}

func (w *Application) makePod() *Pod {
	s := w.Spec
	return NewPod(PodSpec{
		ServiceAccount: s.ServiceAccount,
		Node:           s.Node,
		App:            s.App,
		Namespace:      s.Namespace,
		AppType:        s.Type,
		ClusterType:    s.ClusterType,
	})
}

func (w *Application) getSims() []model.Simulation {
	sims := []model.Simulation{w.service}

	if w.sidecar != nil {
		sims = append(sims, w.sidecar)
	}
	if w.envoyFilter != nil {
		sims = append(sims, w.envoyFilter)
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

	if w.virtualService != nil {
		sims = append(sims, w.virtualService)
	}
	if w.destRule != nil {
		sims = append(sims, w.destRule)
	}
	for _, gw := range w.gateways {
		sims = append(sims, gw)
	}
	for _, scr := range w.secrets {
		sims = append(sims, scr)
	}
	for _, p := range w.pods {
		sims = append(sims, p)
	}
	if w.endpoint != nil {
		sims = append(sims, w.endpoint)
	}
	if w.deployment != nil {
		sims = append(sims, w.deployment)
	}
	return sims
}

func (w *Application) Run(ctx model.Context) (err error) {
	return model.AggregateSimulation{Simulations: w.getSims()}.RunParallel(ctx)
}

func (w *Application) Cleanup(ctx model.Context) error {
	return model.AggregateSimulation{Simulations: model.ReverseSimulations(w.getSims())}.CleanupParallel(ctx)
}

func (w *Application) Refresh(ctx model.Context) error {
	// TODO: implement for Deployment
	if len(w.pods) == 0 {
		return nil
	}

	i := 0
	if len(w.pods) > 1 {
		i = rand.IntnRange(0, len(w.pods)-1)
	}

	newPod := w.makePod()
	removed := w.pods[i]

	w.pods[i] = newPod
	if err := newPod.Run(ctx); err != nil {
		return err
	}

	if w.endpoint != nil {
		if err := w.endpoint.SetAddresses(ctx, w.getIps()); err != nil {
			return fmt.Errorf("endpoints: %v", err)
		}
	}

	if err := removed.Cleanup(ctx); err != nil {
		return err
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
