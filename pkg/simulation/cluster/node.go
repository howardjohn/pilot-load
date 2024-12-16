package cluster

import (
	"errors"
	"time"

	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/ptr"
	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"
)

type NodeSpec struct {
	Name        string
	Region      string
	Zone        string
	ClusterType model.ClusterType
	Ztunnel     bool
}

type Node struct {
	Spec  *NodeSpec
	uid   types.UID
	start time.Time
	xds   *xds.Simulation
}

var _ model.Simulation = &Node{}

func NewNode(s NodeSpec) *Node {
	return &Node{Spec: &s, start: time.Now()}
}

func (n *Node) Run(ctx model.Context) (err error) {
	if n.Spec.ClusterType == model.Real {
		return nil
	}
	nm, err := kube.ApplyRes(ctx.Client, n.getNode())
	if err != nil {
		return err
	}
	n.uid = nm.GetUID()

	go func() {
		tc := time.After(time.Duration(0))
		for {
			select {
			case <-ctx.Done():
				return
			case <-tc:
				if err := kube.Apply(ctx.Client, n.getLease()); err != nil {
					// fast retry
					tc = time.After(time.Second * 1)
					log.Warnf("lease update for %v failed: %v", n.Spec.Name, err)
				} else {
					tc = time.After(time.Second * 20)
				}
			}
		}
	}()
	if n.Spec.Ztunnel {
		n.xds = &xds.Simulation{
			Labels:    nil,
			Namespace: "istio-system",
			Name:      "ztunnel-" + n.Spec.Name,
			IP:        util.GetIP(),
			AppType:   model.ZtunnelType,
			// TODO: multicluster
			Cluster:  "Kubernetes",
			GrpcOpts: ctx.Args.Auth.GrpcOptions("ztunnel", "istio-system"),
			Delta:    true,
		}
		return n.xds.Run(ctx)
	}
	return nil
}

func (n *Node) Cleanup(ctx model.Context) error {
	if n.Spec.ClusterType == model.Real {
		return nil
	}
	var xdsErr error
	if n.xds != nil {
		xdsErr = n.xds.Cleanup(ctx)
	}
	return errors.Join(
		xdsErr,
		kube.Delete(ctx.Client, n.getNode()),
		kube.Delete(ctx.Client, n.getLease()),
	)
}

func (n *Node) getNode() *v1.Node {
	s := n.Spec
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.Name,
			Labels: map[string]string{
				"topology.kubernetes.io/zone":   s.Zone,
				"topology.kubernetes.io/region": s.Region,
				"kubernetes.io/hostname":        s.Name,
				// Avoid kube-system daemonset getting scheduled
				// Works at least for kind
				//"kubernetes.io/arch":            "amd64",
				//"kubernetes.io/os":              "linux",
				"kubernetes.io/role":       "agent",
				"pilot-load.istio.io/node": "fake",
			},
		},
	}
	if n.Spec.ClusterType == model.FakeNode {
		node.Spec = v1.NodeSpec{
			Taints: []v1.Taint{{
				Key:    "pilot-load.istio.io/node",
				Value:  "fake",
				Effect: v1.TaintEffectNoSchedule,
			}},
		}
		node.Status = v1.NodeStatus{
			Capacity: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewQuantity(32, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(256*1024*1024*1024, resource.BinarySI),
				v1.ResourcePods:   *resource.NewQuantity(255, resource.DecimalSI),
			},
			Allocatable: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewQuantity(32, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(256*1024*1024*1024, resource.BinarySI),
				v1.ResourcePods:   *resource.NewQuantity(255, resource.DecimalSI),
			},
			Phase: v1.NodeRunning,
			Conditions: []v1.NodeCondition{
				{
					Type:               v1.NodeReady,
					Reason:             "KubeletReady",
					Message:            "kubelet is posting ready status",
					Status:             v1.ConditionTrue,
					LastHeartbeatTime:  metav1.NewTime(time.Now()),
					LastTransitionTime: metav1.NewTime(n.start),
				},
			},
			Addresses:       nil,
			DaemonEndpoints: v1.NodeDaemonEndpoints{},
			NodeInfo: v1.NodeSystemInfo{
				KubeletVersion:   "fake",
				KubeProxyVersion: "fake",
				OperatingSystem:  "linux",
				Architecture:     "amd64",
			},
		}
	}
	return node
}

func (n *Node) getLease() *coordinationv1.Lease {
	s := n.Spec
	return &coordinationv1.Lease{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Name,
			Namespace: "kube-node-lease",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Node",
				Name:       s.Name,
				UID:        n.uid,
			}},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &s.Name,
			LeaseDurationSeconds: ptr.Of(int32(40)),
			RenewTime:            ptr.Of(metav1.NewMicroTime(time.Now())),
		},
	}
}
