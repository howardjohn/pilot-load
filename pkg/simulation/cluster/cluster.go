package cluster

import (
	"fmt"

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
}

var _ model.Simulation = &Cluster{}

func NewCluster(s ClusterSpec) *Cluster {
	cluster := &Cluster{Name: "primary", Spec: &s}

	for _, ns := range s.Config.Namespaces {
		for r := 0; r < ns.Replicas; r++ {
			cluster.namespaces = append(cluster.namespaces, NewNamespace(NamespaceSpec{
				Name:        fmt.Sprintf("%s-%s", util.StringDefault(ns.Name, "namespace"), util.GenUID()),
				Deployments: ns.Deployments,
			}))
		}
	}
	return cluster
}

func (c *Cluster) getSims() []model.Simulation {
	sims := []model.Simulation{}
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
	return model.AggregateSimulation{n.getSims()}.Cleanup(ctx)
}
