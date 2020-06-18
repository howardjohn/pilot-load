package cluster

import (
	"fmt"

	"github.com/howardjohn/pilot-load/pkg/simulation/app"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type NamespaceSpec struct {
	Name      string
	Workloads int
}

type Namespace struct {
	Spec      *NamespaceSpec
	ns        *app.Namespace
	sa        map[string]*app.ServiceAccount
	workloads []*app.Workload
}

var _ model.Simulation = &Namespace{}

func NewNamespace(s NamespaceSpec) *Namespace {
	ns := &Namespace{Spec: &s}

	ns.ns = app.NewNamespace(app.NamespaceSpec{
		Name: "workload",
	})
	ns.sa = map[string]*app.ServiceAccount{
		"default": app.NewServiceAccount(app.ServiceAccountSpec{
			Namespace: ns.Spec.Name,
			Name:      "default",
		}),
	}
	for i := 0; i < s.Workloads; i++ {
		ns.workloads = append(ns.workloads, app.NewWorkload(app.WorkloadSpec{
			App:            fmt.Sprintf("app-%d", i),
			Node:           "node",
			Namespace:      ns.Spec.Name,
			ServiceAccount: "default",
			Instances:      5,
		}))
	}
	return ns
}

func (n *Namespace) getSims() []model.Simulation {
	sims := []model.Simulation{n.ns}
	for _, sa := range n.sa {
		sims = append(sims, sa)
	}
	for _, w := range n.workloads {
		sims = append(sims, w)
	}
	return sims
}

func (n *Namespace) Run(ctx model.Context) error {
	return model.AggregateSimulation{n.getSims()}.Run(ctx)
}

func (n *Namespace) Cleanup(ctx model.Context) error {
	return model.AggregateSimulation{n.getSims()}.Cleanup(ctx)
}
