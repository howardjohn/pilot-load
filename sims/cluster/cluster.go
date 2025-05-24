package cluster

import (
	"fmt"
	"math/rand"
	"runtime"
	"time"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/app"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/kubetypes"
	"istio.io/istio/pkg/log"
)

type ClusterSpec struct {
	Config Config
}

type Cluster struct {
	Name       string
	Spec       *ClusterSpec
	namespaces []*Namespace
	nodes      []*Node
	running    chan struct{}
}

func (c *Cluster) GetConfig() any {
	return c.Spec.Config
}

var _ model.Simulation = &Cluster{}

func NewCluster(s ClusterSpec) *Cluster {
	cluster := &Cluster{Name: "primary", Spec: &s, running: make(chan struct{})}

	needNodes := s.Config.PodCount() / 255
	if s.Config.NodeCount() < needNodes {
		log.Fatalf("have %d nodes, but need %d for %d pods", s.Config.NodeCount(), needNodes, s.Config.PodCount())
	}
	for _, node := range s.Config.Nodes {
		for r := 0; r < node.Count; r++ {
			cluster.nodes = append(cluster.nodes, NewNode(NodeSpec{
				Name:    fmt.Sprintf("%s-%s", node.Name, util.GenUID()),
				Region:  "region",
				Zone:    "zone",
				Ztunnel: node.Ztunnel != nil,
			}))
		}
	}

	for nsId, ns := range s.Config.Namespaces {
		for r := 0; r < ns.Replicas; r++ {
			deployments := ns.Applications
			for i, d := range ns.Applications {
				d.GetNode = cluster.SelectNode
				deployments[i] = d
			}
			name := util.StringDefault(ns.Name, "namespace")
			if ns.Replicas > 1 {
				name = fmt.Sprintf("%s-%s", name, util.GenUIDOrStableIdentifier(s.Config.StableNames, nsId, r))
			}
			cluster.namespaces = append(cluster.namespaces, NewNamespace(NamespaceSpec{
				Name:                name,
				Deployments:         deployments,
				TemplateDefinitions: s.Config.Templates,
				Templates:           ns.Templates,
				StableNames:         s.Config.StableNames,
				Waypoint:            ns.Waypoint,
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
			for _, cfg := range w.GetConfigs() {
				if model.IsRefreshable(cfg) {
					cfgs = append(cfgs, cfg)
				}
			}
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

	sims = append(sims, c.getIstioResources()...)

	for _, ns := range c.namespaces {
		sims = append(sims, ns)
	}
	return sims
}

func (c *Cluster) Run(ctx model.Context) error {
	// Act as kubelet
	// TODO: make a leader election mechanism for multi-instance
	go c.watchPods(ctx)
	nodes := []model.Simulation{}
	for _, ns := range c.nodes {
		nodes = append(nodes, ns)
	}
	if err := (model.AggregateSimulation{Simulations: nodes}.Run(ctx)); err != nil {
		return fmt.Errorf("failed to bootstrap nodes: %v", err)
	}

	istioResources := c.getIstioResources()
	if err := (model.AggregateSimulation{Simulations: istioResources}.Run(ctx)); err != nil {
		return fmt.Errorf("failed to bootstrap istio resources: %v", err)
	}

	total := len(c.namespaces)
	for i, ns := range c.namespaces {
		log.Infof("starting namespace %v (%d of %d)", ns.Spec.Name, i+1, total)
		if err := (model.AggregateSimulation{Simulations: []model.Simulation{ns}}.Run(ctx)); err != nil {
			return fmt.Errorf("failed to bootstrap namespace: %v", err)
		}
		select {
		case <-time.After(time.Duration(c.Spec.Config.GracePeriod)):
		case <-ctx.Done():
			return nil
		}
	}

	log.Infof("cluster %q synced, starting cluster scaler", c.Name)
	close(c.running)
	return (&ClusterScaler{Cluster: c}).Run(ctx)
}

func (c *Cluster) Running() chan struct{} {
	return c.running
}

func (c *Cluster) Cleanup(ctx model.Context) error {
	return model.AggregateSimulation{Simulations: model.ReverseSimulations(c.getSims())}.CleanupParallel(ctx)
}

func (c *Cluster) watchPods(ctx model.Context) {
	pods := kclient.NewFiltered[*v1.Pod](ctx.Client, kubetypes.Filter{
		ObjectTransform: StripPodUnusedFields,
	})
	q := NewQueue("pods",
		WithWorkers(runtime.GOMAXPROCS(0)),
		WithReconciler(func(key types.NamespacedName) error {
			p := pods.Get(key.Name, key.Namespace)
			if p == nil {
				return nil
			}
			if p.Spec.NodeSelector["pilot-load.istio.io/node"] != "fake" {
				// not our pod
				return nil
			}
			if p.DeletionTimestamp != nil {
				if err := pods.Delete(p.Name, p.Namespace); controllers.IgnoreNotFound(err) != nil {
					return fmt.Errorf("delete: %v", err)
				}
				return nil
			}
			if p.Status.Phase == v1.PodRunning {
				// no action needed
				return nil
			}
			if p.Spec.NodeName == "" {
				// Not yet ready
				return nil
			}
			p = p.DeepCopy()
			p.Status.Phase = v1.PodRunning
			p.Status.Conditions = nil
			p.Status.Conditions = append(p.Status.Conditions, v1.PodCondition{
				Type:               v1.PodReady,
				Status:             v1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(time.Now()),
			})
			p.Status.Conditions = append(p.Status.Conditions, v1.PodCondition{
				Type:               v1.ContainersReady,
				Status:             v1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(time.Now()),
			})
			p.Status.PodIP = util.GetIP()
			p.Status.ContainerStatuses = make([]v1.ContainerStatus, len(p.Spec.Containers))
			for i, c := range p.Spec.Containers {
				p.Status.ContainerStatuses[i] = v1.ContainerStatus{
					Name: c.Name,
					State: v1.ContainerState{
						Running: &v1.ContainerStateRunning{StartedAt: metav1.NewTime(time.Now())},
					},
					Ready: true,
					Image: c.Image,
				}
			}
			p.Spec = v1.PodSpec{}
			if err := kube.ApplyStatusRealSSA(ctx.Client, p); err != nil {
				return fmt.Errorf("apply status: %v", err)
			}
			return nil
		}),
		WithMaxAttempts(5))
	pods.AddEventHandler(controllers.ObjectHandler(q.AddObject))
	pods.Start(ctx.Done())
	q.Run(ctx.Done())
}

func (c *Cluster) getIstioResources() []model.Simulation {
	sims := []model.Simulation{}

	return sims
}

func StripPodUnusedFields(obj any) (any, error) {
	t, ok := obj.(metav1.ObjectMetaAccessor)
	if !ok {
		// shouldn't happen
		return obj, nil
	}
	t.GetObjectMeta().SetManagedFields(nil)
	t.GetObjectMeta().SetAnnotations(nil)
	t.GetObjectMeta().SetLabels(nil)
	// only container ports can be used
	if pod := obj.(*v1.Pod); pod != nil {
		containers := []v1.Container{}
		for _, c := range pod.Spec.Containers {
			containers = append(containers, v1.Container{
				Name:  c.Name,
				Image: c.Image,
			})
		}
		oldSpec := pod.Spec
		newSpec := v1.PodSpec{
			NodeSelector:       oldSpec.NodeSelector,
			Containers:         containers,
			ServiceAccountName: oldSpec.ServiceAccountName,
			NodeName:           oldSpec.NodeName,
		}
		pod.Spec = newSpec
		pod.Status.Conditions = nil
		pod.Status.InitContainerStatuses = nil
		pod.Status.ContainerStatuses = nil
	}

	return obj, nil
}
