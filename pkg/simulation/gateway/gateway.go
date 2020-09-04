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
	"istio.io/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/simulation/cluster"
	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

var scope = log.RegisterScope("probe", "", 0)

type ProberSpec struct {
	Delay      time.Duration
	Replicas   int
	GatewayUrl string
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
	prober int
	ttl      time.Duration
	attempts int
	err      error
}

func (p *ProberSimulation) Run(ctx model.Context) error {
	sims := []model.Simulation{}
	sims = append(sims, cluster.NewKubernetesNamespace(cluster.KubernetesNamespaceSpec{Name: namespace, RealCluster: true}))
	sims = append(sims, config.NewGeneric(createGateway()))
	p.Simulations = sims

	if err := (model.AggregateSimulation{p.Simulations}.Run(ctx)); err != nil {
		return err
	}

	t0 := time.Now()
	results := make(chan proberStatus, p.Spec.Replicas)
	for i := 0; i < p.Spec.Replicas; i++ {
		i := i
		vs := config.NewGeneric(createVirtualService(i))
		p.Simulations = append(p.Simulations, vs)
		// TODO start proper
		go func() {
			if err := vs.Run(ctx); err != nil {
				log.Errorf("failed to run virtual service: %v", err)
			}
			log.Infof("starting prober %d", i)
			res := runProbe(ctx, "104.197.141.217", i)
			res.prober = i
			results <- res
		}()
		time.Sleep(p.Spec.Delay)
	}

	finals := []proberStatus{}

	for i := 0; i < p.Spec.Replicas; i++ {
		got := <-results
		finals = append(finals, got)
	}
	sort.SliceStable(finals, func(i, j int) bool {
		return finals[i].prober < finals[j].prober
	})

	log.Infof("Total test time: %v", time.Since(t0))
	logResults(finals)
	writeCsv(finals)
	ctx.Cancel()
	return nil
}

func writeCsv(finals []proberStatus) {
	f, err := ioutil.TempFile("/tmp", "")
	if err != nil {
		log.Errorf("failed to write csv: %v", err)
	}
	sb := strings.Builder{}
	sb.WriteString("prober,ttl,attempts\n")
	for _, r := range finals {
		sb.WriteString(fmt.Sprintf("%d,%d,%d\n", r.prober, r.ttl.Milliseconds(), r.attempts))
	}
	if err := ioutil.WriteFile(f.Name(), []byte(sb.String()), 0644); err != nil {
		log.Errorf("failed to write csv: %v")
	}
	log.Infof("wrote csv results to %v", f.Name())
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
	log.Infof("Completed %d probes", len(ttls))
	log.Infof("Total requests: %v", attempts)
	log.Infof("Average requests until success: %v", attempts/len(ttls))
	log.Infof("Average TTL: %v", sum(ttls)/time.Duration(len(ttls)))
	log.Infof("Max TTL: %v", max(ttls))
	for _, r := range res {
		if r.err != nil {
			log.Errorf("Got error: %v", r.err)
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
		resp, err := client.Do(req)
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
