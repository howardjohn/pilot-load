package cluster

import (
	"fmt"
	"strings"

	"github.com/howardjohn/pilot-load/pkg/simulation/app"
	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
)

type NamespaceSpec struct {
	Name                string
	TemplateDefinitions model.TemplateDefinitions
	Deployments         []model.ApplicationConfig
	StableNames         bool
	Waypoint            string
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

	nsLabels := map[string]string{
		"istio-injection": "enabled",
	}
	if s.Waypoint != "" {
		ns, name, ok := strings.Cut(s.Waypoint, "/")
		if ok {
			nsLabels["istio.io/use-waypoint"] = name + "-static"
			nsLabels["istio.io/use-waypoint-namespace"] = ns
		} else {
			nsLabels["istio.io/use-waypoint"] = s.Waypoint + "-static"
		}
	}
	ns.ns = NewKubernetesNamespace(KubernetesNamespaceSpec{
		Name:   s.Name,
		Labels: nsLabels,
	})
	// Explicitly make a service account, sometimes its too slow to make one...
	ns.sa = map[string]*app.ServiceAccount{
		"default": app.NewServiceAccount(app.ServiceAccountSpec{
			Namespace: ns.Spec.Name,
			Name:      "default",
		}),
	}

	for idx, d := range s.Deployments {
		for r := range d.Replicas {
			suffix := util.GenUIDOrStableIdentifier(s.StableNames, idx, r)
			if d.Type == model.WaypointType {
				suffix = "static"
			}
			ns.deployments = append(ns.deployments, ns.createApplication(d, suffix))
		}
	}
	return ns
}

func (n *Namespace) createApplication(args model.ApplicationConfig, suffix string) *app.Application {
	return app.NewApplication(app.ApplicationSpec{
		App:       fmt.Sprintf("%s-%s", util.StringDefault(args.Name, "app"), suffix),
		Node:      args.GetNode,
		Namespace: n.Spec.Name,
		// TODO implement different service accounts
		ServiceAccount:      "default",
		Instances:           args.Instances,
		Type:                args.Type,
		GatewayConfig:       args.Gateways,
		Templates:           args.Templates,
		TemplateDefinitions: n.Spec.TemplateDefinitions,
		Labels:              args.Labels,
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
	return model.AggregateSimulation{Simulations: model.ReverseSimulations(n.getSims())}.Cleanup(ctx)
}
