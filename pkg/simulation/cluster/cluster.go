package cluster

import (
	"fmt"
	"math/rand"
	"runtime"
	"time"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/app"
	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/log"
)

type ClusterSpec struct {
	Config model.ClusterConfig
}

type Cluster struct {
	Name                  string
	Spec                  *ClusterSpec
	namespaces            []*Namespace
	envoyFilter           *config.EnvoyFilter
	sidecar               *config.Sidecar
	telemetry             *config.Telemetry
	peerAuthentication    *config.PeerAuthentication
	requestAuthentication *config.RequestAuthentication
	authorizationPolicy   *config.AuthorizationPolicy
	nodes                 []*Node
	running               chan struct{}
}

var _ model.Simulation = &Cluster{}

func NewCluster(s ClusterSpec) *Cluster {
	cluster := &Cluster{Name: "primary", Spec: &s, running: make(chan struct{})}

	if s.Config.ClusterType == model.FakeNode {
		needNodes := s.Config.PodCount() / 255
		if s.Config.NodeCount() < needNodes {
			log.Fatalf("have %d nodes, but need %d for %d pods", s.Config.NodeCount(), needNodes, s.Config.PodCount())
		}
	}
	for _, node := range s.Config.Nodes {
		for r := 0; r < node.Count; r++ {
			cluster.nodes = append(cluster.nodes, NewNode(NodeSpec{
				Name:        fmt.Sprintf("%s-%s", node.Name, util.GenUID()),
				Region:      "region",
				Zone:        "zone",
				ClusterType: s.Config.ClusterType,
			}))
		}
	}

	if s.Config.Istio.Default || s.Config.Istio.EnvoyFilter != nil {
		cluster.envoyFilter = config.NewEnvoyFilter(config.EnvoyFilterSpec{
			Namespace: "istio-system",
			APIScope:  model.Global,
		})
	}
	if s.Config.Istio.Default || s.Config.Istio.Sidecar != nil {
		cluster.sidecar = config.NewSidecar(config.SidecarSpec{
			Namespace: "istio-system",
			APIScope:  model.Global,
		})
	}
	if s.Config.Istio.Default || s.Config.Istio.Telemetry != nil {
		cluster.telemetry = config.NewTelemetry(config.TelemetrySpec{
			Namespace: "istio-system",
			APIScope:  model.Global,
		})
	}
	if s.Config.Istio.Default || s.Config.Istio.RequestAuthentication != nil {
		cluster.requestAuthentication = config.NewRequestAuthentication(config.RequestAuthenticationSpec{
			Namespace: "istio-system",
			APIScope:  model.Global,
		})
	}
	if s.Config.Istio.Default || s.Config.Istio.PeerAuthentication != nil {
		cluster.peerAuthentication = config.NewPeerAuthentication(config.PeerAuthenticationSpec{
			Namespace: "istio-system",
			APIScope:  model.Global,
		})
	}
	if s.Config.Istio.Default || s.Config.Istio.AuthorizationPolicy != nil {
		cluster.authorizationPolicy = config.NewAuthorizationPolicy(config.AuthorizationPolicySpec{
			Namespace: "istio-system",
			APIScope:  model.Global,
		})
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
				ClusterType: s.Config.ClusterType,
				Istio:       ns.Istio,
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

	sims = append(sims, c.getIstioResources()...)

	for _, ns := range c.namespaces {
		sims = append(sims, ns)
	}
	return sims
}

func (c *Cluster) Run(ctx model.Context) error {
	if c.Spec.Config.ClusterType == model.FakeNode {
		// Act as kubelet
		go c.watchPods(ctx)
	}
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
	pods := kclient.New[*v1.Pod](ctx.Client)
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
				if err := pods.Delete(p.Name, p.Namespace); err != nil {
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

	if c.sidecar != nil {
		sims = append(sims, c.sidecar)
	}
	if c.envoyFilter != nil {
		sims = append(sims, c.envoyFilter)
	}
	if c.telemetry != nil {
		sims = append(sims, c.telemetry)
	}
	if c.authorizationPolicy != nil {
		sims = append(sims, c.authorizationPolicy)
	}
	if c.peerAuthentication != nil {
		sims = append(sims, c.peerAuthentication)
	}
	if c.requestAuthentication != nil {
		sims = append(sims, c.requestAuthentication)
	}

	return sims
}
