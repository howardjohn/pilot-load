package simulation

import (
	"fmt"
	"log"
	"time"
)

type WorkloadSpec struct {
	App            string
	Node           string
	Namespace      string
	ServiceAccount string
	Instances      int
	Scaling        *ScalerSpec
}

type Workload struct {
	Spec     *WorkloadSpec
	endpoint *Endpoint
	pods     []*Pod
	service  *Service
	vservice *VirtualService
	scaler   *Scaler
}

var _ Simulation = &Workload{}

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
		IP:        getIp(),
	})
	w.vservice = NewVirtualService(VirtualServiceSpec{
		App:       s.App,
		Namespace: s.Namespace,
	})
	if w.Spec.Scaling != nil {
		w.Spec.Scaling.scaler = w.Scale
		w.scaler = NewScaler(*w.Spec.Scaling)
	}
	return w
}

func NewScaler(s ScalerSpec) *Scaler {
	return &Scaler{Spec: &s}
}

type ScalerSpec struct {
	scaler   func(ctx Context, n int) error
	start    int
	stop     int
	step     int
	interval time.Duration
}

type Scaler struct {
	Spec *ScalerSpec
}

func (s Scaler) Run(ctx Context) error {
	errCh := make(chan error)
	cur := s.Spec.start
	tick := time.NewTicker(s.Spec.interval)
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return err
		case <-tick.C:
			cur += s.Spec.step
			scaleTo := cur
			if s.Spec.stop != 0 && s.Spec.stop < scaleTo {
				return nil
			}
			go func() {
				if err := s.Spec.scaler(ctx, scaleTo); err != nil {
					errCh <- err
				}
			}()
		}
	}
}

var _ Simulation = &Scaler{}

func (w Workload) Run(ctx Context) (err error) {
	sims := []Simulation{w.service, w.endpoint, w.vservice}
	if w.scaler != nil {
		sims = append(sims, w.scaler)
	}
	for _, p := range w.pods {
		sims = append(sims, p)
	}
	agg := NewAggregateSimulation(nil, sims)
	if err := agg.Run(ctx); err != nil {
		return fmt.Errorf("workload: %v", err)
	}
	return nil
}

func (w *Workload) Scale(ctx Context, n int) error {
	log.Println("scaling pod from", len(w.pods), "->", n)
	if n < len(w.pods) {
		log.Println("cannot scale down yet")
		return nil
	}
	newSims := []Simulation{}
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
	if err := w.endpoint.SetAddresses(w.getIps()); err != nil {
		return fmt.Errorf("endpoints: %v", err)
	}
	if err := NewAggregateSimulation(nil, newSims).Run(ctx); err != nil {
		return fmt.Errorf("scale: %v", err)
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
