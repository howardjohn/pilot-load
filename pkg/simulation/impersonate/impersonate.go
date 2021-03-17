package impersonate

import (
	"time"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"

	"istio.io/pkg/log"
)

type ImpersonateSpec struct {
	Selector model.Selector
	Delay    time.Duration
	Replicas int
}

type ImpersonateSimulation struct {
	Spec ImpersonateSpec
	done []chan struct{}
}

var _ model.Simulation = &ImpersonateSimulation{}

func NewSimulation(spec ImpersonateSpec) *ImpersonateSimulation {
	return &ImpersonateSimulation{Spec: spec}
}

func (i *ImpersonateSimulation) Run(ctx model.Context) error {
	informers := ctx.Client.Informers()
	pods, _ := informers.Core().V1().Pods(), informers.Core().V1().Pods().Informer()
	informers.Start(ctx.Done())
	informers.WaitForCacheSync(ctx.Done())
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
			pod := pod
			done := make(chan struct{})
			i.done = append(i.done, done)
			ip := pod.Status.PodIP

			xsim := xds.Simulation{
				Labels:    pod.Labels,
				Namespace: pod.Namespace,
				Name:      pod.Name,
				IP:        ip,
				Cluster:   "",
				PodType:   "",
				GrpcOpts:  ctx.Args.Auth.GrpcOptions(pod.Spec.ServiceAccountName, pod.Namespace),
			}
			log.Infof("Starting pod %v/%v (%v), replica %d", pod.Name, pod.Namespace, ip, n)
			go func() {
				xsim.Run(ctx)
				close(done)
			}()
			time.Sleep(i.Spec.Delay)
		}
	}
	log.Infof("All pods started (%d total)", total)
	return nil
}

func (i *ImpersonateSimulation) Cleanup(ctx model.Context) error {
	// Wait for terminations
	for _, d := range i.done {
		<-d
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
