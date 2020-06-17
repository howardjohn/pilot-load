package app

import (
	"log"

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
	vservice *VirtualService
}

var _ model.Simulation = &Workload{}

func NewWorkload(s WorkloadSpec) *Workload {
	w := &Workload{Spec: &s}

	for i := 0; i < s.Instances; i++ {
		w.pods = append(w.pods, NewPod(PodSpec{
			ServiceAccount: s.ServiceAccount,
			Node:           s.Node,
			App:            s.App,
			Namespace:      s.Namespace,
		}))
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
	w.vservice = NewVirtualService(VirtualServiceSpec{
		App:       s.App,
		Namespace: s.Namespace,
	})
	return w
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
	// TODO implement this
	log.Println("scaling pod from", len(w.pods), "->", n)
	if n < len(w.pods) {
		log.Println("cannot scale down yet")
		return nil
	}
	newSims := []model.Simulation{}
	for n > len(w.pods) {
		pod := NewPod(PodSpec{
			ServiceAccount: w.Spec.ServiceAccount,
			Node:           w.Spec.Node,
			App:            w.Spec.App,
			Namespace:      w.Spec.Namespace,
		})
		w.pods = append(w.pods, pod)
		newSims = append(newSims, pod)
	}

	// TODO this should be a simulation maybe?
	//if err := w.endpoint.SetAddresses(w.getIps()); err != nil {
	//	return fmt.Errorf("endpoints: %v", err)
	//}
	//if err := simulation.NewAggregateSimulation(nil, newSims).Run(ctx); err != nil {
	//	return fmt.Errorf("scale: %v", err)
	//}
	return nil
}

func (w Workload) getIps() []string {
	ret := []string{}
	for _, p := range w.pods {
		ret = append(ret, p.Spec.IP)
	}
	return ret
}
