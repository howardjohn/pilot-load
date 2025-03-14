package app

import (
	"istio.io/istio/pkg/log"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type ApplicationSpec struct {
	App            string
	Node           func() string
	Namespace      string
	ServiceAccount string
	Instances      int
	Type           model.AppType
	GatewayConfig  model.GatewayConfig
	Istio          model.IstioApplicationConfig
	Labels         map[string]string
}

type Application struct {
	Spec                  *ApplicationSpec
	pods                  []*Pod
	service               *Service
	kgateways             []*config.KubeGateway
	virtualService        *config.VirtualService
	httpRoute             *config.HTTPRoute
	gateways              []*config.Gateway
	secrets               []*config.Secret
	destRule              *config.DestinationRule
	workloadEntry         *config.WorkloadEntry
	workloadGroup         *config.WorkloadGroup
	serviceEntry          *config.ServiceEntry
	envoyFilter           *config.EnvoyFilter
	sidecar               *config.Sidecar
	telemetry             *config.Telemetry
	peerAuthentication    *config.PeerAuthentication
	requestAuthentication *config.RequestAuthentication
	authorizationPolicy   *config.AuthorizationPolicy
}

var (
	_ model.Simulation            = &Application{}
	_ model.ScalableSimulation    = &Application{}
	_ model.RefreshableSimulation = &Application{}
)

func NewApplication(s ApplicationSpec) *Application {
	w := &Application{Spec: &s}

	// Apply common CRDs to all app types
	if s.Istio.Default || s.Istio.VirtualService != nil {
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
	if s.Istio.HttpRoutes != nil {
		gateways := s.Istio.HttpRoutes.Gateways
		w.httpRoute = config.NewHTTPRoute(config.HTTPRouteSpec{
			App:       s.App,
			Namespace: s.Namespace,
			Gateways:  gateways,
		})
	}
	if s.Istio.Default || s.Istio.DestinationRule != nil {
		w.destRule = config.NewDestinationRule(config.DestinationRuleSpec{
			App:       s.App,
			Namespace: s.Namespace,
			Subsets:   []string{"a"},
		})
	}
	if s.Istio.Default || s.Istio.Telemetry != nil {
		w.telemetry = config.NewTelemetry(config.TelemetrySpec{
			App:       s.App,
			Namespace: s.Namespace,
			APIScope:  model.Application,
		})
	}
	if s.Istio.Default || s.Istio.RequestAuthentication != nil {
		w.requestAuthentication = config.NewRequestAuthentication(config.RequestAuthenticationSpec{
			App:       s.App,
			Namespace: s.Namespace,
			APIScope:  model.Application,
		})
	}
	if s.Istio.Default || s.Istio.PeerAuthentication != nil {
		w.peerAuthentication = config.NewPeerAuthentication(config.PeerAuthenticationSpec{
			App:       s.App,
			Namespace: s.Namespace,
			APIScope:  model.Application,
		})
	}
	if s.Istio.Default || s.Istio.AuthorizationPolicy != nil {
		w.authorizationPolicy = config.NewAuthorizationPolicy(config.AuthorizationPolicySpec{
			App:       s.App,
			Namespace: s.Namespace,
			APIScope:  model.Application,
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

	// Apply CRDs for sidecar and GW app type
	if s.Istio.Default || s.Istio.EnvoyFilter != nil {
		w.envoyFilter = config.NewEnvoyFilter(config.EnvoyFilterSpec{
			App:       s.App,
			Namespace: s.Namespace,
			APIScope:  model.Application,
		})
	}
	if s.Istio.Default || s.Istio.Sidecar != nil {
		w.sidecar = config.NewSidecar(config.SidecarSpec{
			App:       s.App,
			Namespace: s.Namespace,
			APIScope:  model.Application,
		})
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

	if s.Type == model.GatewayType {
		for range s.GatewayConfig.Replicas {
			name := s.GatewayConfig.Name
			if s.GatewayConfig.Kubernetes {
				name = s.App
				gw := config.NewKubeGateway(config.KubeGatewaySpec{
					Name:      s.GatewayConfig.Name,
					App:       s.App,
					Namespace: s.Namespace,
				})
				w.kgateways = append(w.kgateways, gw)
			} else {
				gw := config.NewGateway(config.GatewaySpec{
					Name:      s.GatewayConfig.Name,
					App:       s.App,
					Namespace: s.Namespace,
				})
				w.gateways = append(w.gateways, gw)
			}
			w.secrets = append(w.secrets, config.NewSecret(config.SecretSpec{
				Namespace: s.Namespace,
				Name:      name,
			}))
		}
	}

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
	if w.virtualService != nil {
		sims = append(sims, w.virtualService)
	}
	if w.httpRoute != nil {
		sims = append(sims, w.httpRoute)
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
	if w.telemetry != nil {
		sims = append(sims, w.telemetry)
	}
	if w.authorizationPolicy != nil {
		sims = append(sims, w.authorizationPolicy)
	}
	if w.peerAuthentication != nil {
		sims = append(sims, w.peerAuthentication)
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
		Node:           s.Node(),
		App:            s.App,
		Namespace:      s.Namespace,
		AppType:        s.Type,
	})
}

func (w *Application) getSims() []model.Simulation {
	sims := []model.Simulation{}

	if w.service != nil {
		sims = append(sims, w.service)
	}
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
	if w.telemetry != nil {
		sims = append(sims, w.telemetry)
	}
	if w.authorizationPolicy != nil {
		sims = append(sims, w.authorizationPolicy)
	}
	if w.peerAuthentication != nil {
		sims = append(sims, w.peerAuthentication)
	}
	if w.requestAuthentication != nil {
		sims = append(sims, w.requestAuthentication)
	}

	if w.virtualService != nil {
		sims = append(sims, w.virtualService)
	}

	if w.httpRoute != nil {
		sims = append(sims, w.httpRoute)
	}
	if w.destRule != nil {
		sims = append(sims, w.destRule)
	}
	for _, gw := range w.kgateways {
		sims = append(sims, gw)
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

	return nil
}
