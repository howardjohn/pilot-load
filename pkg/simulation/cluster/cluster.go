package cluster

import (
	"fmt"
	"math/rand"

	"istio.io/pkg/log"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
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
			Name:   fmt.Sprintf("node-%s", util.GenUID()),
			Region: "region",
			Zone:   "zone",
		}))
	}
	for _, ns := range s.Config.Namespaces {
		for r := 0; r < ns.Replicas; r++ {
			deployments := ns.Applications
			for i, d := range ns.Applications {
				d.GetNode = cluster.SelectNode
				deployments[i] = d
			}
			cluster.namespaces = append(cluster.namespaces, NewNamespace(NamespaceSpec{
				Name:        fmt.Sprintf("%s-%s", util.StringDefault(ns.Name, "namespace"), util.GenUID()),
				Deployments: deployments,
			}))
		}
	}
	return cluster
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

func (n *Cluster) Run(ctx model.Context) error {
	if err := (model.AggregateSimulation{n.getSims()}.Run(ctx)); err != nil {
		return fmt.Errorf("failed to bootstrap cluster: %v", err)
	}
	log.Infof("cluster %q synced, starting cluster scaler", n.Name)
	return (&ClusterScaler{Cluster: n}).Run(ctx)
}

func (n *Cluster) Cleanup(ctx model.Context) error {
	return model.AggregateSimulation{model.ReverseSimulations(n.getSims())}.Cleanup(ctx)
}
