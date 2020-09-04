package gateway

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"time"

	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	ilog "istio.io/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/cluster"
	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

var scope = ilog.RegisterScope("probe", "", 0)

type ProberSpec struct {
	Delay          time.Duration
	DelayThreshold int
	Replicas       int
	Address        string
}

type ProberSimulation struct {
	Spec        ProberSpec
	Simulations []model.Simulation
}

var _ model.Simulation = &ProberSimulation{}

func NewSimulation(spec ProberSpec) *ProberSimulation {
	return &ProberSimulation{Spec: spec}
}

type proberStatus struct {
	prober   int
	ttl      time.Duration
	attempts int
	err      error
}

func (p *ProberSimulation) Run(ctx model.Context) error {
	if p.Spec.Address == "" {
		return fmt.Errorf("gateway address must be set")
	}
	sims := []model.Simulation{}
	sims = append(sims, cluster.NewKubernetesNamespace(cluster.KubernetesNamespaceSpec{Name: namespace, RealCluster: true}))
	sims = append(sims, config.NewGeneric(createGateway()))
	p.Simulations = sims

	if err := (model.AggregateSimulation{p.Simulations}.Run(ctx)); err != nil {
		return err
	}

	t0 := time.Now()
	results := make(chan proberStatus, p.Spec.Replicas)
	completed := p.Spec.Replicas
	func() {
		for i := 0; i < p.Spec.Replicas; i++ {
			select {
			case <-ctx.Done():
				completed = i - 1
				return
			default:
			}
			i := i
			vs := config.NewGeneric(createVirtualService(i))
			p.Simulations = append(p.Simulations, vs)
			// TODO start proper
			go func() {
				if err := vs.Run(ctx); err != nil {
					scope.Errorf("failed to run virtual service: %v", err)
				}
				scope.Infof("starting prober %d", i)
				res := runProbe(ctx, p.Spec.Address, i)
				res.prober = i
				results <- res
			}()
			if i > p.Spec.DelayThreshold {
				time.Sleep(p.Spec.Delay)
			}
		}
	}()

	finals := []proberStatus{}

	for i := 0; i < completed; i++ {
		got := <-results
		finals = append(finals, got)
	}
	sort.SliceStable(finals, func(i, j int) bool {
		return finals[i].prober < finals[j].prober
	})

	scope.Infof("Total test time: %v", time.Since(t0))
	logResults(finals)
	writeCsv(finals)
	ctx.Cancel()
	return nil
}

func writeCsv(finals []proberStatus) {
	f, err := ioutil.TempFile("/tmp", "")
	if err != nil {
		scope.Errorf("failed to write csv: %v", err)
	}
	sb := strings.Builder{}
	sb.WriteString("prober,ttl,attempts\n")
	for _, r := range finals {
		sb.WriteString(fmt.Sprintf("%d,%d,%d\n", r.prober, r.ttl.Milliseconds(), r.attempts))
	}
	if err := ioutil.WriteFile(f.Name(), []byte(sb.String()), 0644); err != nil {
		scope.Errorf("failed to write csv: %v")
	}
	scope.Infof("wrote csv results to %v", f.Name())
}

func logResults(res []proberStatus) {
	ttls := []time.Duration{}
	attempts := 0
	for _, r := range res {
		ttls = append(ttls, r.ttl)
		attempts += r.attempts
	}
	sort.SliceStable(ttls, func(i, j int) bool {
		return ttls[i] < ttls[j]
	})
	scope.Infof("Completed %d probes", len(ttls))
	scope.Infof("Total requests: %v", attempts)
	scope.Infof("Average requests until success: %v", attempts/len(ttls))
	scope.Infof("Average TTL: %v", sum(ttls)/time.Duration(len(ttls)))
	scope.Infof("Max TTL: %v", max(ttls))
	for _, r := range res {
		if r.err != nil {
			scope.Errorf("Got error: %v", r.err)
		}
	}
}

func sum(ttls []time.Duration) time.Duration {
	s := time.Duration(0)
	for _, t := range ttls {
		s += t
	}
	return s
}

func max(ttls []time.Duration) time.Duration {
	s := time.Duration(0)
	for _, t := range ttls {
		if t > s {
			s = t
		}
	}
	return s
}

var client = http.Client{Timeout: time.Millisecond * 500}

func runProbe(ctx model.Context, gw string, index int) proberStatus {
	attempts := 0
	t0 := time.Now()
	for attempts < 1000 {
		select {
		case <-ctx.Done():
			return proberStatus{err: fmt.Errorf("context canceled")}
		default:
		}
		attempts++
		scope.Debugf("probe %d attempt %d %v", index, attempts, time.Since(t0))
		req, err := http.NewRequest("GET", "http://"+gw, nil)
		if err != nil {
			return proberStatus{err: err}
		}
		req.Host = fmt.Sprintf("vs-%d.example.com", index)
		req.Header.Set("virtual-service", fmt.Sprintf("vs-%d", index))
		t1 := time.Now()
		resp, err := client.Do(req)
		scope.Debugf("probe %d request latency: %v", index, time.Since(t1))
		if err != nil {
			scope.Warnf("probe %d failed on attempt %d: %v", index, attempts, err)
			time.Sleep(time.Millisecond * 10 * time.Duration(attempts))
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 418 {
			delta := time.Since(t0)
			scope.Infof("probe %d attempt %d complete in %v", index, attempts, delta)
			return proberStatus{ttl: delta, attempts: attempts}
		} else {
			scope.Warnf("probe %d failed on attempt %d: status code %d", index, attempts, resp.StatusCode)
			time.Sleep(time.Millisecond * time.Duration(attempts))
			continue
		}
	}
	return proberStatus{err: fmt.Errorf("probe timed out after %d attempts and %v", attempts, time.Since(t0))}
}

func (p *ProberSimulation) Cleanup(ctx model.Context) error {
	return model.AggregateSimulation{model.ReverseSimulations(p.Simulations)}.Cleanup(ctx)
}

const namespace = "gateway-test"

func createVirtualService(index int) *v1alpha3.VirtualService {
	return &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("vs-%d", index),
			Namespace: namespace,
		},
		Spec: networkingv1alpha3.VirtualService{
			Hosts:    []string{fmt.Sprintf("vs-%d.example.com", index)},
			Gateways: []string{fmt.Sprintf("%s/gateway", namespace)},
			Http: []*networkingv1alpha3.HTTPRoute{
				{
					Name: "",
					Match: []*networkingv1alpha3.HTTPMatchRequest{{
						Headers: map[string]*networkingv1alpha3.StringMatch{
							"virtual-service": {MatchType: &networkingv1alpha3.StringMatch_Exact{Exact: fmt.Sprintf("vs-%d", index)}},
						},
					}},
					Route: []*networkingv1alpha3.HTTPRouteDestination{
						{
							Destination: &networkingv1alpha3.Destination{
								Host: "httpbin.org",
							},
						},
					},
					Fault: &networkingv1alpha3.HTTPFaultInjection{
						Abort: &networkingv1alpha3.HTTPFaultInjection_Abort{
							Percentage: &networkingv1alpha3.Percent{Value: 100},
							ErrorType:  &networkingv1alpha3.HTTPFaultInjection_Abort_HttpStatus{HttpStatus: 418},
						},
					},
				},
			},
		},
	}
}
func createGateway() *v1alpha3.Gateway {
	return &v1alpha3.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway",
			Namespace: namespace,
		},
		Spec: networkingv1alpha3.Gateway{
			Servers: []*networkingv1alpha3.Server{
				{
					Port: &networkingv1alpha3.Port{
						Number:   80,
						Name:     "http",
						Protocol: "HTTP",
					},
					Hosts: []string{"*"},
				},
			},
			Selector: map[string]string{
				"istio": "ingressgateway",
			},
		},
	}
}
