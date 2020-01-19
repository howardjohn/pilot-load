package simulation

type WorkloadSpec struct {
	App            string
	Node           string
	Namespace      string
	ServiceAccount string
	Instances      int
}

type Workload struct {
	Spec           *WorkloadSpec
	namespace      *Namespace
	serviceAccount *ServiceAccount
	endpoint       *Endpoint
	pods           []*Pod
	service        *Service
}

var _ Simulation = &Workload{}

func NewWorkload(s WorkloadSpec) *Workload {
	w := &Workload{Spec: &s}
	w.namespace = NewNamespace(NamespaceSpec{
		Name: s.Namespace,
	})
	w.serviceAccount = NewServiceAccount(ServiceAccountSpec{
		App:       s.App,
		Namespace: s.Namespace,
		Name:      s.ServiceAccount,
	})

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
	return w
}

func (w Workload) Run(ctx Context) (err error) {
	sims := []Simulation{w.service, w.endpoint, w.serviceAccount}
	for _, p := range w.pods {
		sims = append(sims, p)
	}
	agg := NewAggregateSimulation([]Simulation{w.namespace}, sims)
	return agg.Run(ctx)
}

func (w Workload) getIps() []string {
	ret := []string{}
	for _, p := range w.pods {
		ret = append(ret, p.Spec.IP)
	}
	return ret
}
