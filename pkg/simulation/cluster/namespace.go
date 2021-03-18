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
}

type Namespace struct {
	Spec        *NamespaceSpec
	ns          *KubernetesNamespace
	sa          map[string]*app.ServiceAccount
	sidecar     *config.Sidecar
	deployments []*app.Application
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
	ns.sidecar = config.NewSidecar(config.SidecarSpec{Namespace: s.Name})
	for _, d := range s.Deployments {
		for r := 0; r < d.Replicas; r++ {
			ns.deployments = append(ns.deployments, ns.createDeployment(d))
		}
	}
	return ns
}

func (n *Namespace) createDeployment(args model.ApplicationConfig) *app.Application {
	return app.NewApplication(app.ApplicationSpec{
		App:       fmt.Sprintf("%s-%s", util.StringDefault(args.Name, "app"), util.GenUID()),
		Node:      args.GetNode(),
		Namespace: n.Spec.Name,
		// TODO implement different service accounts
		ServiceAccount: "default",
		Instances:      args.Instances,
		PodType:        args.PodType,
		GatewayConfig:  args.Gateways,
	})
}

func (n *Namespace) InsertDeployment(ctx model.Context, args model.ApplicationConfig) error {
	wl := n.createDeployment(args)
	n.deployments = append(n.deployments, wl)
	return wl.Run(ctx)
}

func (n *Namespace) getSims() []model.Simulation {
	sims := []model.Simulation{n.ns, n.sidecar}
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
