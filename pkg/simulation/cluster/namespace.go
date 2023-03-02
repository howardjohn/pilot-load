package cluster

import (
	"fmt"

	"github.com/howardjohn/pilot-load/pkg/simulation/app"
	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
)

type NamespaceSpec struct {
	Name        string
	Deployments []model.ApplicationConfig
	ClusterType model.ClusterType
	Istio       model.IstioNSConfig
}

type Namespace struct {
	Spec                  *NamespaceSpec
	ns                    *KubernetesNamespace
	sa                    map[string]*app.ServiceAccount
	envoyFilter           *config.EnvoyFilter
	sidecar               *config.Sidecar
	telemetry             *config.Telemetry
	peerAuthentication    *config.PeerAuthentication
	requestAuthentication *config.RequestAuthentication
	authorizationPolicy   *config.AuthorizationPolicy
	deployments           []*app.Application
}

var _ model.Simulation = &Namespace{}

func NewNamespace(s NamespaceSpec) *Namespace {
	ns := &Namespace{Spec: &s}

	ns.ns = NewKubernetesNamespace(KubernetesNamespaceSpec{
		Name: s.Name,
	})
	ns.sa = map[string]*app.ServiceAccount{
		"default": app.NewServiceAccount(app.ServiceAccountSpec{
			Namespace: ns.Spec.Name,
			Name:      "default",
		}),
	}

	if s.Istio.Default == true || s.Istio.EnvoyFilter != nil {
		ns.envoyFilter = config.NewEnvoyFilter(config.EnvoyFilterSpec{
			Namespace: ns.Spec.Name,
			APIScope:  model.Namespace,
		})
	}
	if s.Istio.Default == true || s.Istio.Sidecar != nil {
		ns.sidecar = config.NewSidecar(config.SidecarSpec{
			Namespace: ns.Spec.Name,
			APIScope:  model.Namespace,
		})
	}
	if s.Istio.Default == true || s.Istio.Telemetry != nil {
		ns.telemetry = config.NewTelemetry(config.TelemetrySpec{
			Namespace: ns.Spec.Name,
			APIScope:  model.Namespace,
		})
	}
	if s.Istio.Default == true || s.Istio.RequestAuthentication != nil {
		ns.requestAuthentication = config.NewRequestAuthentication(config.RequestAuthenticationSpec{
			Namespace: ns.Spec.Name,
			APIScope:  model.Namespace,
		})
	}
	if s.Istio.Default == true || s.Istio.PeerAuthentication != nil {
		ns.peerAuthentication = config.NewPeerAuthentication(config.PeerAuthenticationSpec{
			Namespace: ns.Spec.Name,
			APIScope:  model.Namespace,
		})
	}
	if s.Istio.Default == true || s.Istio.AuthorizationPolicy != nil {
		ns.authorizationPolicy = config.NewAuthorizationPolicy(config.AuthorizationPolicySpec{
			Namespace: ns.Spec.Name,
			APIScope:  model.Namespace,
		})
	}

	for _, d := range s.Deployments {
		for r := 0; r < d.Replicas; r++ {
			ns.deployments = append(ns.deployments, ns.createDeployment(d, s.ClusterType))
		}
	}
	return ns
}

func (n *Namespace) createDeployment(args model.ApplicationConfig, ct model.ClusterType) *app.Application {
	return app.NewApplication(app.ApplicationSpec{
		App:       fmt.Sprintf("%s-%s", util.StringDefault(args.Name, "app"), util.GenUID()),
		Node:      args.GetNode(),
		Namespace: n.Spec.Name,
		// TODO implement different service accounts
		ServiceAccount: "default",
		Instances:      args.Instances,
		Type:           args.Type,
		GatewayConfig:  args.Gateways,
		ClusterType:    ct,
		Istio:          args.Istio,
	})
}

func (n *Namespace) getSims() []model.Simulation {
	sims := []model.Simulation{n.ns}
	if n.sidecar != nil {
		sims = append(sims, n.sidecar)
	}
	if n.envoyFilter != nil {
		sims = append(sims, n.envoyFilter)
	}
	if n.telemetry != nil {
		sims = append(sims, n.telemetry)
	}
	if n.authorizationPolicy != nil {
		sims = append(sims, n.authorizationPolicy)
	}
	if n.peerAuthentication != nil {
		sims = append(sims, n.peerAuthentication)
	}
	if n.requestAuthentication != nil {
		sims = append(sims, n.requestAuthentication)
	}

	for _, sa := range n.sa {
		sims = append(sims, sa)
	}
	for _, w := range n.deployments {
		sims = append(sims, w)
	}
	return sims
}

func (n *Namespace) Run(ctx model.Context) error {
	return model.AggregateSimulation{Simulations: n.getSims()}.Run(ctx)
}

func (n *Namespace) Cleanup(ctx model.Context) error {
	return model.AggregateSimulation{Simulations: n.getSims()}.Cleanup(ctx)
}
