package simulation

import (
	"fmt"
	"sync"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	kubelib "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"

	"github.com/howardjohn/pilot-load/adsc"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type DeterministicSimulation struct{}

func getIstiodAddresses(pods kclient.Client[*v1.Pod]) []string {
	s, _ := klabels.Parse("app=istiod")
	ps := pods.List("istio-system", s)
	res := []string{}
	for _, p := range ps {
		res = append(res, p.Status.PodIP+":15010")
	}
	return res
}

func (d DeterministicSimulation) Run(ctx model.Context) error {
	pods := kclient.New[*v1.Pod](ctx.Client)
	pods.Start(ctx.Done())
	kubelib.WaitForCacheSync("pods", ctx.Done(), pods.HasSynced)
	s, _ := klabels.Parse("security.istio.io/tlsMode")
	plist := pods.List(metav1.NamespaceAll, s)
	total := 0
	addresses := getIstiodAddresses(pods)
	if len(addresses) == 0 {
		return fmt.Errorf("no istiod pods")
	}
	diff := false
	for _, pod := range plist {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		total++
		attempt := 0
		var localDiff string
		for attempt < 10 {
			attempt++
			localDiff = d.checkPod(ctx, pod, addresses)
			if localDiff == "" {
				break
			}
		}
		if localDiff == "" {
			log.Infof("PASS: %v.%v", pod.Name, pod.Namespace)
		} else {
			diff = true
			log.Infof("FAIL: %v.%v: %v", pod.Name, pod.Namespace, localDiff)
		}
	}
	log.Infof("All pods started (%d total)", total)
	ctx.Cancel()
	if diff {
		return fmt.Errorf("found diff")
	}
	return nil
}

func (d DeterministicSimulation) checkPod(ctx model.Context, pod *v1.Pod, addresses []string) string {
	meta := map[string]interface{}{
		"ISTIO_VERSION": "1.10.0",
		"CLUSTER_ID":    "Kubernetes",
		"LABELS":        pod.Labels,
		"NAMESPACE":     pod.Namespace,
	}
	ip := pod.Status.PodIP
	log.Infof("Starting pod %v/%v (%v", pod.Name, pod.Namespace, ip)
	resps := make([]*adsc.Responses, len(addresses))
	wg := sync.WaitGroup{}
	for i, addr := range addresses {
		addr := addr
		i := i
		wg.Add(1)
		go func() {
			res, err := adsc.Fetch(addr, &adsc.Config{
				Namespace:      pod.Namespace,
				Workload:       pod.Name,
				Meta:           meta,
				IP:             ip,
				Context:        ctx,
				StoreResponses: true,
			})
			if err != nil {
				log.Error(err)
				return
			}
			resps[i] = res
			wg.Done()
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		return "timeout"
	}
	baseline := resps[0]
	for i, resp := range resps {
		if resp == nil {
			return "invalid response"
		}
		if i == 0 {
			continue
		}
		if d := compare(baseline.Clusters, resp.Clusters); d != "" {
			return d
		}
		if d := compare(baseline.Listeners, resp.Listeners); d != "" {
			return d
		}
		if d := compare(baseline.Routes, resp.Routes); d != "" {
			return d
		}
		if d := compare(baseline.Endpoints, resp.Endpoints); d != "" {
			return d
		}
	}
	return ""
}

func compare(base, comp map[string]proto.Message) string {
	if len(base) != len(comp) {
		return fmt.Sprintf("mismatched resource count: %v vs %v", len(base), len(comp))
	}
	if len(base) == 0 {
		log.Warnf("empty")
	}
	log.Infof("comparing %v resources", len(base))
	for name, got := range comp {
		want := base[name]
		if diff := cmp.Diff(got, want, protocmp.Transform()); diff != "" {
			return fmt.Sprintf("proto diff: %v", diff)
		}
		gots := marshaler.Format(got)
		wants := marshaler.Format(want)
		if gots != wants {
			return fmt.Sprintf("text diff:\n%v\n%v\n", gots, wants)
		}
	}
	return ""
}

var marshaler = prototext.MarshalOptions{}

func (d DeterministicSimulation) Cleanup(ctx model.Context) error {
	return nil
}

var _ model.Simulation = &DeterministicSimulation{}
