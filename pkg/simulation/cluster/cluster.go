package cluster

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/howardjohn/pilot-load/pkg/simulation/app"
	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/pkg/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
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
}

var _ model.Simulation = &Cluster{}

func NewCluster(s ClusterSpec) *Cluster {
	cluster := &Cluster{Name: "primary", Spec: &s}

	for r := 0; r < s.Config.Nodes; r++ {
		cluster.nodes = append(cluster.nodes, NewNode(NodeSpec{
			Name:        fmt.Sprintf("node-%s", util.GenUID()),
			Region:      "region",
			Zone:        "zone",
			ClusterType: s.Config.ClusterType,
		}))
	}

	if s.Config.Istio.Default == true || s.Config.Istio.EnvoyFilter != nil {
		cluster.envoyFilter = config.NewEnvoyFilter(config.EnvoyFilterSpec{
			Namespace: "istio-system",
			APIScope:  model.Global,
		})
	}
	if s.Config.Istio.Default == true || s.Config.Istio.Sidecar != nil {
		cluster.sidecar = config.NewSidecar(config.SidecarSpec{
			Namespace: "istio-system",
			APIScope:  model.Global,
		})
	}
	if s.Config.Istio.Default == true || s.Config.Istio.Telemetry != nil {
		cluster.telemetry = config.NewTelemetry(config.TelemetrySpec{
			Namespace: "istio-system",
			APIScope:  model.Global,
		})
	}
	if s.Config.Istio.Default == true || s.Config.Istio.RequestAuthentication != nil {
		cluster.requestAuthentication = config.NewRequestAuthentication(config.RequestAuthenticationSpec{
			Namespace: "istio-system",
			APIScope:  model.Global,
		})
	}
	if s.Config.Istio.Default == true || s.Config.Istio.PeerAuthentication != nil {
		cluster.peerAuthentication = config.NewPeerAuthentication(config.PeerAuthenticationSpec{
			Namespace: "istio-system",
			APIScope:  model.Global,
		})
	}
	if s.Config.Istio.Default == true || s.Config.Istio.AuthorizationPolicy != nil {
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
		// Only need for deployments. Currently we never use this.
		// go c.watchPods(ctx)
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
	return (&ClusterScaler{Cluster: c}).Run(ctx)
}

func (c *Cluster) Cleanup(ctx model.Context) error {
	return model.AggregateSimulation{Simulations: model.ReverseSimulations(c.getSims())}.CleanupParallel(ctx)
}

func (c *Cluster) watchPods(ctx model.Context) {
	inf := informers.NewSharedInformerFactoryWithOptions(ctx.Client.Kubernetes, 0)
	podInformer := inf.Core().V1().Pods().Informer()
	podLister := inf.Core().V1().Pods().Lister()
	inf.Start(ctx.Done())
	inf.WaitForCacheSync(ctx.Done())
	q := controllers.NewQueue("pods",
		controllers.WithReconciler(func(key types.NamespacedName) error {
			p, _ := podLister.Pods(key.Namespace).Get(key.Name)
			if p == nil {
				return nil
			}
			if p.Spec.NodeSelector["pilot-load.istio.io/node"] != "fake" {
				// not our pod
				return nil
			}
			if p.DeletionTimestamp != nil {
				if err := ctx.Client.Delete(p); err != nil {
					return fmt.Errorf("delete: %v", err)
				}
				return nil
			}
			if p.Status.Phase == v1.PodRunning {
				// no action needed
				return nil
			}
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
					Ready:        true,
					RestartCount: 0,
					Image:        c.Image,
					ImageID:      "",
					ContainerID:  "",
					Started:      nil,
				}
			}
			if err := ctx.Client.ApplyStatus(p); err != nil {
				return fmt.Errorf("apply status: %v", err)
			}
			return nil
		}),
		controllers.WithMaxAttempts(5))
	podInformer.AddEventHandler(controllers.ObjectHandler(q.AddObject))
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
