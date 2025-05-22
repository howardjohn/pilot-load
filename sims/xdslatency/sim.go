package xdslatency

import (
	"context"
	"fmt"
	"time"

	"github.com/howardjohn/pilot-load/adsc"
	"github.com/howardjohn/pilot-load/pkg/flag"
	"github.com/howardjohn/pilot-load/pkg/simulation/config"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	v1 "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/log"
)

type StartupConfig struct {
	Namespace   string
	Concurrency int
	Inject      bool
	Cooldown    time.Duration
	Spec        string
}

func Command(f *pflag.FlagSet) flag.Command {
	startupConfig := model.StartupConfig{
		Concurrency: 1,
		Cooldown:    time.Millisecond * 10,
	}

	var specFile string
	flag.Register(f, &startupConfig.Inject, "inject", "if true, we will inject the pod")
	flag.Register(f, &startupConfig.Concurrency, "concurrency", "number of pods to start concurrently")
	flag.Register(f, &startupConfig.Namespace, "namespace", "namespace to run in")
	flag.Register(f, &startupConfig.Cooldown, "cooldown", "time to wait after starting each pod (per worker)")
	flag.Register(f, &specFile, "spec", "pod spec")
	return flag.Command{
		Name:        "xds-latency",
		Description: "measure end to end XDS latency",
		Build: func(args model.Args) (model.DebuggableSimulation, error) {
			opts := args.Auth.GrpcOptions("default", "default")
			return &XdsLatencySimulation{
				Namespace: "default",
				Name:      "adsc",
				IP:        util.GetIP(),
				Cluster:   "Kubernetes",
				GrpcOpts:  opts,
			}, nil
		},
	}
}

type XdsLatencySimulation struct {
	Labels    map[string]string
	Namespace string
	Name      string
	IP        string
	// Defaults to "Kubernetes"
	Cluster string
	AppType model.AppType

	GrpcOpts []grpc.DialOption

	cancel context.CancelFunc
	done   chan struct{}
}

func (x XdsLatencySimulation) GetConfig() any {
	return x
}

var _ model.Simulation = &XdsLatencySimulation{}

func clone(m map[string]string) map[string]interface{} {
	n := map[string]interface{}{}
	for k, v := range m {
		n[k] = v
	}
	return n
}

func (x XdsLatencySimulation) Run(ctx model.Context) error {
	c, cancel := context.WithCancel(ctx.Context)
	x.cancel = cancel
	x.done = make(chan struct{})
	cluster := x.Cluster
	if cluster == "" {
		cluster = "Kubernetes"
	}
	meta := clone(ctx.Args.Metadata)
	meta["ISTIO_VERSION"] = "1.20.0-pilot-load"
	meta["CLUSTER_ID"] = cluster
	meta["LABELS"] = x.Labels
	meta["NAMESPACE"] = x.Namespace
	meta["SDS"] = "true"
	updates := make(chan string, 10)
	go func() {
		adsc.Connect(ctx.Args.PilotAddress, &adsc.Config{
			Namespace: x.Namespace,
			Workload:  x.Name + "-" + x.IP,
			Meta:      meta,
			NodeType:  string(x.AppType),
			IP:        x.IP,
			Context:   c,
			GrpcOpts:  x.GrpcOpts,
			Updates:   updates,
		})
		close(x.done)
	}()
	i := 0
	var cc model.Simulation
	defer func() { _ = cc.Cleanup(ctx) }()
	for {
		i++
		t0 := time.Now()
		cfg := config.NewGeneric(createConfig(i))
		cc = cfg
		drain(updates)
		if err := cfg.Run(ctx); err != nil {
			return err
		}
		t1 := time.Now()
		if err := getEvent(ctx, updates, "cds"); err != nil {
			return err
		}
		log.Errorf("completed %d in %v %v", i, time.Since(t0), time.Since(t1))
		time.Sleep(time.Millisecond * 250)
		if err := cfg.Cleanup(ctx); err != nil {
			return err
		}
		if err := getEvent(ctx, updates, "cds"); err != nil {
			return err
		}
		time.Sleep(time.Millisecond * 250)
	}
}

func drain(updates chan string) {
	for {
		select {
		case <-updates:
		default:
			return
		}
	}
}

func getEvent(ctx model.Context, updates chan string, s string) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cancel")
		case u := <-updates:
			log.Debugf("Got update: %v", u)
			if u == s {
				return nil
			}
			if u == "close" {
				return fmt.Errorf("close event")
			}
		}
	}
}

func (x XdsLatencySimulation) Cleanup(ctx model.Context) error {
	if x.cancel != nil {
		x.cancel()
	}
	if x.done != nil {
		<-x.done
	}
	return nil
}

func createConfig(index int) *v1.ServiceEntry {
	return &v1.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cfg-%d", index),
			Namespace: "default",
		},
		Spec: networkingv1alpha3.ServiceEntry{
			Hosts: []string{fmt.Sprintf("%d.example.com", index)},
		},
	}
}
