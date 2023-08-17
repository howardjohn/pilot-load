package isolated

import (
	"github.com/howardjohn/pilot-load/pkg/simulation/cluster"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type IsolatedSpec struct {
	Config model.ClusterConfig
}

type Isolated struct {
	Spec    *IsolatedSpec
	Cluster *cluster.Cluster
}

var _ model.Simulation = &Isolated{}

func NewCluster(s IsolatedSpec) *Isolated {
	is := &Isolated{Spec: &s}
	c := cluster.NewCluster(cluster.ClusterSpec{
		Config: s.Config,
	})
	is.Cluster = c

	return is
}

func (c *Isolated) Run(ctx model.Context) error {
	return c.Cluster.Run(ctx)
}

func (c *Isolated) Cleanup(ctx model.Context) error {
	return c.Cluster.Cleanup(ctx)
}
