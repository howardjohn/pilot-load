package impersonate

import (
	"strings"
	"time"

	"istio.io/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	klabels "k8s.io/apimachinery/pkg/labels"

	"github.com/howardjohn/pilot-load/adsc"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
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
			total++
			pod := pod
			meta := map[string]interface{}{
				"ISTIO_VERSION": "1.7.0",
				"CLUSTER_ID":    "Kubernetes",
				"LABELS":        pod.Labels,
				"NAMESPACE":     pod.Namespace,
				"SDS":           "true",
			}
			done := make(chan struct{})
			i.done = append(i.done, done)
			ip := pod.Status.PodIP
			log.Infof("Starting pod %v/%v (%v), replica %d", pod.Name, pod.Namespace, ip, n)
			go func() {
				adsc.Connect(ctx.Args.PilotAddress, &adsc.Config{
					Namespace: pod.Namespace,
					Workload:  pod.Name,
					Meta:      meta,
					IP:        ip,
					Context:   ctx,

					SystemCerts: strings.HasSuffix(ctx.Args.PilotAddress, ":443"),
				})
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
