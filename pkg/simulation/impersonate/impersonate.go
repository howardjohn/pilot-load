package impersonate

import (
	"time"

	"istio.io/istio/pkg/config"
	kubelib "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/log"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"
)

type ImpersonateSpec struct {
	Selector model.Selector
	Delay    time.Duration
	Replicas int
}

type ImpersonateSimulation struct {
	Spec ImpersonateSpec

	knownPods map[types.NamespacedName][]*xds.Simulation

	pods  kclient.Client[*v1.Pod]
	queue controllers.Queue
}

var _ model.Simulation = &ImpersonateSimulation{}

func NewSimulation(spec ImpersonateSpec) *ImpersonateSimulation {
	return &ImpersonateSimulation{
		Spec:      spec,
		knownPods: map[types.NamespacedName][]*xds.Simulation{},
	}
}

func (i *ImpersonateSimulation) Run(ctx model.Context) error {
	sel := getLabelSelector(i.Spec.Selector)
	i.pods = kclient.New[*v1.Pod](ctx.Client)
	i.pods.Start(ctx.Done())
	i.queue = controllers.NewQueue("pods", controllers.WithReconciler(func(key types.NamespacedName) error {
		pod := i.pods.Get(key.Name, key.Namespace)
		if pod == nil {
			i.del(ctx, key)
			return nil
		}
		if pod.Status.PodIP == "" {
			// Need a pod IP before we can watch
			i.del(ctx, key)
			return nil
		}
		selected := sel.Matches(klabels.Set(pod.GetLabels()))
		if selected {
			i.add(ctx, pod)
		} else {
			i.del(ctx, key)
		}
		return nil
	}))
	i.pods.AddEventHandler(controllers.ObjectHandler(i.queue.AddObject))
	// Wait until pods are queued up.
	kubelib.WaitForCacheSync("pods", ctx.Done(), i.pods.HasSynced)
	go i.queue.Run(ctx.Done())

	// Now wait until initial pods are established.
	kubelib.WaitForCacheSync("queue", ctx.Done(), i.queue.HasSynced)

	return nil
}

func newSimulation(ctx model.Context, pod *corev1.Pod) *xds.Simulation {
	return &xds.Simulation{
		Labels:    pod.Labels,
		Namespace: pod.Namespace,
		Name:      pod.Name,
		IP:        pod.Status.PodIP,
		Cluster:   "",
		AppType:   "",
		GrpcOpts:  ctx.Args.Auth.GrpcOptions(pod.Spec.ServiceAccountName, pod.Namespace),
		Delta:     ctx.Args.DeltaXDS,
	}
}

func (i *ImpersonateSimulation) add(ctx model.Context, pod *corev1.Pod) {
	key := config.NamespacedName(pod)
	if _, f := i.knownPods[key]; f {
		// Pod already found, no updates. In theory we could replace it but too complex
		return
	}

	xsims := make([]*xds.Simulation, 0, i.Spec.Replicas)
	for n := 1; n <= i.Spec.Replicas; n++ {
		xsim := newSimulation(ctx, pod)
		log.Infof("Starting pod %v/%v (%v), replica %d", pod.Name, pod.Namespace, xsim.IP, n)
		go func() {
			if err := xsim.Run(ctx); err != nil {
				log.Errorf("failed running %v: %v", xsim.IP, err)
			}
		}()
		xsims = append(xsims, xsim)
		time.Sleep(i.Spec.Delay)
	}
	i.knownPods[key] = xsims
}

func (i *ImpersonateSimulation) del(ctx model.Context, key types.NamespacedName) {
	p, f := i.knownPods[key]
	if !f {
		// Pod not found, nothing to do.
		return
	}
	for _, d := range p {
		err := d.Cleanup(ctx)
		if err != nil {
			log.Error(err)
		}
	}
	delete(i.knownPods, key)
}

func (i *ImpersonateSimulation) Cleanup(ctx model.Context) error {
	<-i.queue.Closed()
	if len(i.knownPods) != 0 {
		for _, p := range i.knownPods {
			for _, d := range p {
				err := d.Cleanup(ctx)
				if err != nil {
					log.Error(err)
				}
			}
		}
	}
	return nil
}

func getLabelSelector(selector model.Selector) klabels.Selector {
	switch selector {
	case model.SidecarSelector:
		s, _ := klabels.Parse("security.istio.io/tlsMode")
		return s
	case model.ExternalSelector:
		s, _ := klabels.Parse("!security.istio.io/tlsMode")
		return s
	case model.BothSelector:
		return klabels.Everything()
	}
	panic("invalid selector " + string(selector))
}
