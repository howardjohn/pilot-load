package cluster

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/howardjohn/pilot-load/pkg/simulation/app"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"

	"istio.io/pkg/log"
)

type ClusterSpec struct {
	Config model.ClusterConfig
}

type Cluster struct {
	Name       string
	Spec       *ClusterSpec
	namespaces []*Namespace
	nodes      []*Node
}

var _ model.Simulation = &Cluster{}

func NewCluster(s ClusterSpec) *Cluster {
	cluster := &Cluster{Name: "primary", Spec: &s}

	for r := 0; r < s.Config.Nodes; r++ {
		cluster.nodes = append(cluster.nodes, NewNode(NodeSpec{
			Name:        fmt.Sprintf("node-%s", util.GenUID()),
			Region:      "region",
			Zone:        "zone",
			RealCluster: s.Config.RealCluster,
		}))
	}
	for _, ns := range s.Config.Namespaces {
		for r := 0; r < ns.Replicas; r++ {
			deployments := ns.Applications
			for i, d := range ns.Applications {
				d.GetNode = cluster.SelectNode
				deployments[i] = d
			}
			name := util.StringDefault(ns.Name, "namespace")
			if ns.Replicas > 1 {
				name = fmt.Sprintf("%s-%s", name, util.GenUID())
			}
			cluster.namespaces = append(cluster.namespaces, NewNamespace(NamespaceSpec{
				Name:        name,
				Deployments: deployments,
				RealCluster: s.Config.RealCluster,
			}))
		}
	}
	return cluster
}

func (c *Cluster) GetRefreshableInstances() []*app.Application {
	var wls []*app.Application
	for _, ns := range c.namespaces {
		wls = append(wls, ns.deployments...)
	}
	return wls
}

func (c *Cluster) GetRefreshableConfig() []model.RefreshableSimulation {
	var cfgs []model.RefreshableSimulation
	for _, ns := range c.namespaces {
		for _, w := range ns.deployments {
			cfgs = append(cfgs, w.GetConfigs()...)
		}
	}
	return cfgs
}

func (c *Cluster) GetRefreshableSecrets() []model.RefreshableSimulation {
	var cfgs []model.RefreshableSimulation
	for _, ns := range c.namespaces {
		for _, w := range ns.deployments {
			cfgs = append(cfgs, w.GetSecrets()...)
		}
	}
	return cfgs
}

// Return a random node
func (c *Cluster) SelectNode() string {
	return c.nodes[rand.Intn(len(c.nodes))].Spec.Name
}

func (c *Cluster) getSims() []model.Simulation {
	sims := []model.Simulation{}
	for _, ns := range c.nodes {
		sims = append(sims, ns)
	}
	for _, ns := range c.namespaces {
		sims = append(sims, ns)
	}
	return sims
}

func (c *Cluster) Run(ctx model.Context) error {
	nodes := []model.Simulation{}
	for _, ns := range c.nodes {
		nodes = append(nodes, ns)
	}
	if err := (model.AggregateSimulation{Simulations: nodes}.Run(ctx)); err != nil {
		return fmt.Errorf("failed to bootstrap nodes: %v", err)
	}

	total := len(c.namespaces)
	for i, ns := range c.namespaces {
		log.Infof("starting namespace %v (%d of %d)", ns.Spec.Name, i+1, total)
		if err := (model.AggregateSimulation{Simulations: []model.Simulation{ns}}.Run(ctx)); err != nil {
			return fmt.Errorf("failed to bootstrap nodes: %v", err)
		}
		select {
		case <-time.After(time.Duration(c.Spec.Config.GracePeriod)):
		case <-ctx.Done():
			return nil
		}
	}

	log.Infof("cluster %q synced, starting cluster scaler", c.Name)
	return (&ClusterScaler{Cluster: c}).Run(ctx)
}

func (c *Cluster) Cleanup(ctx model.Context) error {
	return model.AggregateSimulation{Simulations: model.ReverseSimulations(c.getSims())}.CleanupParallel(ctx)
}
