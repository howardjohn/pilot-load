package app

import (
	"fmt"

	"istio.io/pkg/log"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
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
		IP:        util.GetIP(),
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

func (w *Workload) Scale(ctx model.Context, n int) error {
	log.Infof("%v: scaling pod from %d -> %d", w.Spec.App, len(w.pods), n)
	for n < len(w.pods) {
		i := rand.IntnRange(0, len(w.pods)-1)
		// Remove the element at index i from a.
		old := w.pods[i]
		w.pods[i] = w.pods[len(w.pods)-1] // Copy last element to index i.
		w.pods[len(w.pods)-1] = nil       // Erase last element (write zero value).
		w.pods = w.pods[:len(w.pods)-1]   // Truncate slice.
		log.Infof("terminate pod %v", old.Name())
		if err := old.Cleanup(ctx); err != nil {
			log.Infof("err: %v", err)
			return err
		}
	}

	for n > len(w.pods) {
		pod := w.makePod()
		w.pods = append(w.pods, w.makePod())
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
