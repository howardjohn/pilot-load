package reproducecluster

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/pflag"
	clientnetworkingalpha "istio.io/client-go/pkg/apis/networking/v1alpha3"
	clientnetworkingbeta "istio.io/client-go/pkg/apis/networking/v1beta1"
	clientsecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	clienttelemetry "istio.io/client-go/pkg/apis/telemetry/v1alpha1"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/collections"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/resource"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/util/sets"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	kubescheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/howardjohn/pilot-load/pkg/flag"
	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"
)

type Config struct {
	ConfigFile string
	ConfigOnly bool
	Delay      time.Duration
}

func Command(f *pflag.FlagSet) flag.Command {
	cfg := Config{}

	flag.Register(f, &cfg.Delay, "delay", "delay between each connection")
	flag.RegisterShort(f, &cfg.ConfigOnly, "config-only", "n", "only apply config file, do not connect to XDS")
	flag.RegisterShort(f, &cfg.ConfigFile, "file", "f", "config file")
	return flag.Command{
		Name:        "reproduce-cluster",
		Description: "simulate a cluster by applying the configuration. Makes XDS connections where one would exist in-cluster.",
		Details:     "Expected format: `kubectl get vs,gw,dr,sidecar,svc,endpoints,pod,namespace,sa -oyaml -A | kubectl grep`",
		Build: func(args model.Args) (model.DebuggableSimulation, error) {
			return &ReproduceSimulation{Spec: cfg, running: make(chan struct{})}, nil
		},
	}
}

type ApiDetails struct {
	gvk        schema.GroupVersionKind
	isIstioApi bool
}

type ReproduceSimulation struct {
	Spec    Config
	sims    []model.Simulation
	running chan struct{}
}

func (i *ReproduceSimulation) GetConfig() any {
	return i.Spec
}

var _ model.Simulation = &ReproduceSimulation{}

func toK8s(g config.GroupVersionKind) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   g.Group,
		Version: g.Version,
		Kind:    g.Kind,
	}
}

var order = []ApiDetails{
	{toK8s(gvk.Namespace), false},
	{toK8s(gvk.EnvoyFilter), true},
	{toK8s(gvk.Telemetry), true},
	{toK8s(gvk.ServiceEntry), true},
	{toK8s(gvk.PeerAuthentication), true},
	{toK8s(gvk.Sidecar), true},
	{toK8s(gvk.VirtualService), true},
	{toK8s(gvk.Gateway), true},
	{toK8s(gvk.DestinationRule), true},
	{toK8s(gvk.AuthorizationPolicy), true},
	{toK8s(gvk.RequestAuthentication), true},
	{toK8s(gvk.WorkloadEntry), true},
	{toK8s(gvk.WorkloadGroup), true},
	{toK8s(gvk.ConfigMap), false},
	{toK8s(gvk.Service), false},
	{toK8s(gvk.Endpoints), false},
	{toK8s(config.GroupVersionKind{Group: "", Version: "v1", Kind: "ServiceAccount"}), false},
	{toK8s(gvk.Pod), false},
}

var denylistNamespaces = sets.New("kube-system", "kube-public", "istio-system", "resource-group-system")

func (i *ReproduceSimulation) Run(ctx model.Context) error {
	cfgsByKind, err := parseInputs(i.Spec.ConfigFile)
	if err != nil {
		return err
	}
	total := 0
	for _, g := range order {
		cfg := cfgsByKind[g.gvk]
		for _, c := range cfg {
			if util.IsDone(ctx) {
				return nil
			}
			co := c.DeepCopyObject().(metav1.Object)
			ns := co.GetNamespace()
			name := co.GetName()
			kind := c.GetObjectKind().GroupVersionKind().Kind
			if shouldSkipResource(ns, name, kind, g.isIstioApi) {
				continue
			}

			if kind == gvk.Pod.Kind && !i.Spec.ConfigOnly {
				pod := co.(*v1.Pod)
				x := &xds.Simulation{
					Labels:         co.GetLabels(),
					Namespace:      co.GetNamespace(),
					Name:           co.GetName(),
					ServiceAccount: pod.Spec.ServiceAccountName,
					IP:             pod.Status.PodIP,
					AppType:        model.SidecarType,
					Cluster:        "Kubernetes",
					GrpcOpts:       ctx.Args.Auth.GrpcOptions(pod.Spec.ServiceAccountName, co.GetNamespace()),
					Delta:          ctx.Args.DeltaXDS,
				}
				i.sims = append(i.sims, x)
				if err := x.Run(ctx); err != nil {
					return err
				}
				util.ContextSleep(ctx, i.Spec.Delay)
				continue
			}

			co.SetResourceVersion("")
			co.SetManagedFields(nil)
			co.SetCreationTimestamp(metav1.Time{})
			co.SetFinalizers(nil)
			if svc, ok := co.(*v1.Service); ok {
				// Mutate Service
				spec := svc.Spec
				// Wipe out Cluster IP, we can get one assigned
				if spec.ClusterIP != "None" {
					spec.ClusterIP = ""
					spec.ClusterIPs = nil
				}
				// Same impact, a lot cheaper
				if spec.Type == v1.ServiceTypeLoadBalancer {
					spec.Type = v1.ServiceTypeNodePort
				}
				// We managed endpoint ourself
				spec.Selector = nil
				svc.Spec = spec
			}
			if ep, ok := co.(*v1.Endpoints); ok {
				subsets := ep.Subsets
				for i := range subsets {
					for a := range subsets[i].Addresses {
						// Pod won't exist, so wipe it out
						subsets[i].Addresses[a].TargetRef = nil
					}
				}
				ep.Subsets = subsets
			}
			if sa, ok := co.(*v1.ServiceAccount); ok {
				// Annotations can configure dependencies like WI
				sa.SetAnnotations(nil)
				sa.SetLabels(nil)
			}
			s := newCreateSim(co.(controllers.Object))
			i.sims = append(i.sims, s)
			if err := s.Run(ctx); err != nil {
				// Ignore errors
				log.Errorf("failed to create resource: %v", err)
			}
			if s.skipCleanup {
				log.Infof("already exists: %v/%v.%v", kind, name, ns)
			} else {
				total++
				log.Infof("created: %v/%v.%v", kind, name, ns)
			}
		}
	}
	log.Infof("All configs create (%d total)", total)
	close(i.running)
	return nil
}

func (i *ReproduceSimulation) Cleanup(ctx model.Context) error {
	return model.AggregateSimulation{Simulations: model.ReverseSimulations(i.sims)}.Cleanup(ctx)
}

func (i *ReproduceSimulation) Running() chan struct{} {
	return i.running
}

func parseInputs(inputFile string) (map[schema.GroupVersionKind][]runtime.Object, error) {
	f, err := os.Open(inputFile)
	if err != nil {
		return nil, err
	}
	codecs := serializer.NewCodecFactory(IstioScheme)
	deserializer := codecs.UniversalDeserializer()

	reader := yaml.NewYAMLReader(bufio.NewReader(f))
	resp := map[schema.GroupVersionKind][]runtime.Object{}
	for {
		chunk, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		var obj runtime.Object
		obj, _, err = deserializer.Decode(chunk, nil, obj)
		if err != nil {
			return nil, fmt.Errorf("cannot parse message: %v", err)
		}
		gvk := obj.GetObjectKind().GroupVersionKind()

		// Convert v1beta1 apiversions to v1alpha3 for Istio networking APIs
		s, exists := collections.PilotGatewayAPI().FindByGroupVersionAliasesKind(resource.FromKubernetesGVK(&gvk))
		if exists {
			obj.GetObjectKind().SetGroupVersionKind(s.GroupVersionKind().Kubernetes())
			gvk = obj.GetObjectKind().GroupVersionKind()
		}

		resp[gvk] = append(resp[gvk], obj)
	}

	return resp, nil
}

// IstioScheme returns a scheme will all known Istio-related types added
var IstioScheme = func() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(kubescheme.AddToScheme(scheme))
	utilruntime.Must(clientnetworkingalpha.AddToScheme(scheme))
	utilruntime.Must(clientnetworkingbeta.AddToScheme(scheme))
	utilruntime.Must(clientsecurity.AddToScheme(scheme))
	utilruntime.Must(clienttelemetry.AddToScheme(scheme))
	return scheme
}()

type createSim[T controllers.Object] struct {
	Spec        T
	skipCleanup bool
}

var _ model.Simulation = &createSim[controllers.Object]{}

func newCreateSim[T controllers.Object](s T) *createSim[T] {
	return &createSim[T]{Spec: s}
}

func (v *createSim[T]) Run(ctx model.Context) (err error) {
	created, err := kube.Create(ctx.Client, v.Spec)
	if err != nil {
		return err
	}
	if !created {
		v.skipCleanup = true
	}
	return nil
}

func (v *createSim[T]) Cleanup(ctx model.Context) error {
	if v.skipCleanup {
		return nil
	}

	log.Infof("cleaning up %v/%v.%v", v.Spec.GetObjectKind().GroupVersionKind().Kind, v.Spec.GetName(), v.Spec.GetNamespace())
	return kube.Delete(ctx.Client, v.Spec)
}

func shouldSkipResource(ns string, name string, kind string, isIstioApi bool) bool {
	if denylistNamespaces.Contains(ns) && !isIstioApi { // Allow Istio APIs to created, valid usecase is root namespace
		return true
	}
	if ns == "default" && name == "kubernetes" && kind == "Service" { // Skip the Kubernetes Service
		return true
	}
	if kind == gvk.Namespace.Kind && denylistNamespaces.Contains(name) {
		return true
	}
	return false
}
