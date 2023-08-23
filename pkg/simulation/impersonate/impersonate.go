package impersonate

import (
	"sync"
	"time"

	"istio.io/pkg/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"
)

type ImpersonateSpec struct {
	Selector model.Selector
	Delay    time.Duration
	Replicas int
	Watch    bool
}

type ImpersonateSimulation struct {
	Spec ImpersonateSpec
	done []chan struct{}

	podsMut sync.Mutex
	pods    map[types.UID][]*xds.Simulation
}

var _ model.Simulation = &ImpersonateSimulation{}

func NewSimulation(spec ImpersonateSpec) *ImpersonateSimulation {
	return &ImpersonateSimulation{
		Spec: spec,
		pods: map[types.UID][]*xds.Simulation{},
	}
}

func (i *ImpersonateSimulation) Run(ctx model.Context) error {
	informers := ctx.Client.Informers()
	if i.Spec.Watch {
		return i.watch(ctx, informers)
	}
	return i.list(ctx, informers)
}

func (i *ImpersonateSimulation) list(ctx model.Context, informers informers.SharedInformerFactory) error {
	informers.Start(ctx.Done())
	informers.WaitForCacheSync(ctx.Done())
	pods := informers.Core().V1().Pods()
	plist, err := pods.Lister().Pods(metav1.NamespaceAll).List(getLabelSelector(i.Spec.Selector))
	if err != nil {
		return err
	}
	total := 0
	for n := 1; n <= i.Spec.Replicas; n++ {
		for _, pod := range plist {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			total++
			xsim := newSimulation(ctx, pod)
			done := make(chan struct{})
			i.done = append(i.done, done)
			log.Infof("Starting pod %v/%v (%v), replica %d", pod.Name, pod.Namespace, xsim.IP, n)
			go func() {
				if err := xsim.Run(ctx); err != nil {
					log.Errorf("failed running %v: %v", xsim.IP, err)
				}
				close(done)
			}()
			time.Sleep(i.Spec.Delay)
		}
	}
	log.Infof("All pods started (%d total)", total)
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

func (i *ImpersonateSimulation) watch(ctx model.Context, informers informers.SharedInformerFactory) error {
	sel := getLabelSelector(i.Spec.Selector)

	pods := informers.Core().V1().Pods()
	_, err := pods.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod).DeepCopy()
			if !sel.Matches(klabels.Set(pod.Labels)) {
				return
			}
			i.add(ctx, pod)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			pod := newObj.(*corev1.Pod).DeepCopy()
			if !sel.Matches(klabels.Set(pod.Labels)) {
				i.del(ctx, pod)
			} else {
				i.add(ctx, pod)
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod).DeepCopy()
			i.del(ctx, pod)
		},
	})
	if err != nil {
		return err
	}

	informers.Start(ctx.Done())
	return nil
}

func (i *ImpersonateSimulation) add(ctx model.Context, pod *corev1.Pod) {
	i.podsMut.Lock()
	defer i.podsMut.Unlock()
	p := i.pods[pod.UID]
	if p != nil {
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
	i.pods[pod.UID] = xsims
}

func (i *ImpersonateSimulation) del(ctx model.Context, pod *corev1.Pod) {
	i.podsMut.Lock()
	defer i.podsMut.Unlock()

	p := i.pods[pod.UID]
	if p == nil {
		return
	}
	for _, d := range p {
		err := d.Cleanup(ctx)
		if err != nil {
			log.Error(err)
		}
	}
	delete(i.pods, pod.UID)
}

func (i *ImpersonateSimulation) Cleanup(ctx model.Context) error {
	// Wait for terminations
	for _, d := range i.done {
		<-d
	}

	i.podsMut.Lock()
	defer i.podsMut.Unlock()
	if len(i.pods) != 0 {
		for _, p := range i.pods {
			for _, d := range p {
				err := d.Cleanup(ctx)
				if err != nil {
					log.Error(err)
				}
			}
		}
		i.pods = map[types.UID][]*xds.Simulation{}
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
