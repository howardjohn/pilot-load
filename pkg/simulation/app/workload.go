package app

import (
	"fmt"

	"istio.io/pkg/log"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type WorkloadSpec struct {
	App            string
	Node           string
	Namespace      string
	ServiceAccount string
	Instances      int
}

type Workload struct {
	Spec     *WorkloadSpec
	endpoint *Endpoint
	pods     []*Pod
	service  *Service
	vservice *config.VirtualService
}

var _ model.Simulation = &Workload{}
var _ model.ScalableSimulation = &Workload{}
var _ model.RefreshableSimulation = &Workload{}

func NewWorkload(s WorkloadSpec) *Workload {
	w := &Workload{Spec: &s}

	for i := 0; i < s.Instances; i++ {
		w.pods = append(w.pods, w.makePod())
	}

	w.endpoint = NewEndpoint(EndpointSpec{
		Node:      s.Node,
		App:       s.App,
		Namespace: s.Namespace,
		IPs:       w.getIps(),
	})
	w.service = NewService(ServiceSpec{
		App:       s.App,
		Namespace: s.Namespace,
	})
	w.vservice = config.NewVirtualService(config.VirtualServiceSpec{
		App:       s.App,
		Namespace: s.Namespace,
	})
	return w
}

func (w *Workload) makePod() *Pod {
	s := w.Spec
	return NewPod(PodSpec{
		ServiceAccount: s.ServiceAccount,
		Node:           s.Node,
		App:            s.App,
		Namespace:      s.Namespace,
	})
}

func (w *Workload) getSims() []model.Simulation {
	sims := []model.Simulation{w.service, w.endpoint, w.vservice}
	for _, p := range w.pods {
		sims = append(sims, p)
	}
	return sims
}

func (w *Workload) Run(ctx model.Context) (err error) {
	return model.AggregateSimulation{w.getSims()}.Run(ctx)
}

func (w *Workload) Cleanup(ctx model.Context) error {
	return model.AggregateSimulation{w.getSims()}.Cleanup(ctx)
}

func (w *Workload) Refresh(ctx model.Context) error {
	if err := w.Scale(ctx, -1); err != nil {
		return err
	}
	return w.Scale(ctx, 1)
}

func (w *Workload) Scale(ctx model.Context, delta int) error {
	return w.ScaleTo(ctx, len(w.pods)+delta)
}

func (w *Workload) ScaleTo(ctx model.Context, n int) error {
	log.Infof("%v: scaling pod from %d -> %d", w.Spec.App, len(w.pods), n)
	for n < len(w.pods) && n >= 0 {
		i := 0
		if len(w.pods) > 1 {
			i = rand.IntnRange(0, len(w.pods)-1)
		}
		// Remove the element at index i from a.
		old := w.pods[i]
		w.pods[i] = w.pods[len(w.pods)-1] // Copy last element to index i.
		w.pods[len(w.pods)-1] = nil       // Erase last element (write zero value).
		w.pods = w.pods[:len(w.pods)-1]   // Truncate slice.
		if err := old.Cleanup(ctx); err != nil {
			log.Infof("err: %v", err)
			return err
		}
	}

	for n > len(w.pods) {
		pod := w.makePod()
		w.pods = append(w.pods, pod)
		if err := pod.Run(ctx); err != nil {
			return err
		}
	}

	if err := w.endpoint.SetAddresses(ctx, w.getIps()); err != nil {
		return fmt.Errorf("endpoints: %v", err)
	}
	return nil
}

func (w Workload) getIps() []string {
	ret := []string{}
	for _, p := range w.pods {
		ret = append(ret, p.Spec.IP)
	}
	return ret
}
