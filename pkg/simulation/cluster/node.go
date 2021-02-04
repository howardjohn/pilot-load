package cluster

import (
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodeSpec struct {
	Name   string
	Region string
	Zone   string
}

type Node struct {
	Spec *NodeSpec
}

var _ model.Simulation = &Node{}

func NewNode(s NodeSpec) *Node {
	return &Node{Spec: &s}
}

func (n *Node) Run(ctx model.Context) (err error) {
	return ctx.Client.Apply(n.getNode())
}

func (n *Node) Cleanup(ctx model.Context) error {
	return ctx.Client.Delete(n.getNode())
}

func (n *Node) getNode() *v1.Node {
	s := n.Spec
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.Name,
			Labels: map[string]string{
				"topology.kubernetes.io/zone":   s.Zone,
				"topology.kubernetes.io/region": s.Region,
			},
		},
	}
}
