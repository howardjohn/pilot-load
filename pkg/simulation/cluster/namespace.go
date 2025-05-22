package cluster

import (
	"fmt"
	"strings"

	"istio.io/istio/pkg/maps"

	"github.com/howardjohn/pilot-load/pkg/simulation/app"
	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
)

type NamespaceSpec struct {
	Name                string
	TemplateDefinitions model.TemplateDefinitions
	Deployments         []model.ApplicationConfig
	Templates           []model.ConfigTemplate
	StableNames         bool
	Waypoint            string
}

type Namespace struct {
	Spec        *NamespaceSpec
	ns          *KubernetesNamespace
	sa          map[string]*app.ServiceAccount
	deployments []*app.Application
	configs     []*config.Templated
}

var _ model.Simulation = &Namespace{}

func NewNamespace(s NamespaceSpec) *Namespace {
	ns := &Namespace{Spec: &s}

	nsLabels := map[string]string{
		"istio-injection": "enabled",
	}

	for _, tmpl := range s.Templates {
		cfg := maps.Clone(tmpl.Config)
		if cfg == nil {
			cfg = map[string]any{}
		}
		cfg[config.Namespace] = s.Name
		ns.configs = append(ns.configs, config.NewTemplated(config.TemplatedSpec{
			Template: s.TemplateDefinitions.Get(tmpl.Name),
			Config:   cfg,
			Refresh:  tmpl.Refresh,
		}))
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
		Templates:           args.Templates,
		TemplateDefinitions: n.Spec.TemplateDefinitions,
		Labels:              args.Labels,
	})
}

func (n *Namespace) getSims() []model.Simulation {
	sims := []model.Simulation{n.ns}

	for _, cfg := range n.configs {
		sims = append(sims, cfg)
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
