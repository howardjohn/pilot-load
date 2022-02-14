package reproduce

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	kubescheme "k8s.io/client-go/kubernetes/scheme"

	clientnetworkingalpha "istio.io/client-go/pkg/apis/networking/v1alpha3"
	clientnetworkingbeta "istio.io/client-go/pkg/apis/networking/v1beta1"
	clientsecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	clienttelemetry "istio.io/client-go/pkg/apis/telemetry/v1alpha1"
	"istio.io/istio/pilot/pkg/util/sets"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/pkg/log"
)

type ReproduceSpec struct {
	Delay      time.Duration
	ConfigFile string
	ConfigOnly bool
}

type ReproduceSimulation struct {
	Spec ReproduceSpec
	sims []model.Simulation
}

var _ model.Simulation = &ReproduceSimulation{}

func NewSimulation(spec ReproduceSpec) *ReproduceSimulation {
	return &ReproduceSimulation{Spec: spec}
}

func toK8s(g config.GroupVersionKind) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   g.Group,
		Version: g.Version,
		Kind:    g.Kind,
	}
}

var order = []schema.GroupVersionKind{
	toK8s(gvk.Namespace),
	toK8s(gvk.Sidecar),
	toK8s(gvk.VirtualService),
	toK8s(gvk.Gateway),
	toK8s(gvk.DestinationRule),
	toK8s(gvk.Service),
	toK8s(gvk.Endpoints),
	toK8s(config.GroupVersionKind{Group: "", Version: "v1", Kind: "ServiceAccount"}),
	toK8s(gvk.Pod),
}

var denylistNamespaces = sets.NewSet("kube-system", "kube-public", "istio-system", "resource-group-system")

func (i *ReproduceSimulation) Run(ctx model.Context) error {
	cfgsByKind, err := parseInputs(i.Spec.ConfigFile)
	if err != nil {
		return err
	}
	total := 0
	for _, g := range order {
		cfgs := cfgsByKind[g]
		for _, c := range cfgs {
			if util.IsDone(ctx) {
				return nil
			}
			co := c.DeepCopyObject().(metav1.Object)
			ns := co.GetNamespace()
			name := co.GetName()
			kind := c.GetObjectKind().GroupVersionKind().Kind
			if denylistNamespaces.Contains(ns) {
				continue
			}
			if ns == "default" && name == "kubernetes" && kind == "Service" { // Skip the Kubernetes Service
				continue
			}
			if kind == gvk.Namespace.Kind && denylistNamespaces.Contains(name) {
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
					PodType:        model.SidecarType,
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
			s := newCreateSim(co.(kube.Object))
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
	return nil
}

func (i *ReproduceSimulation) Cleanup(ctx model.Context) error {
	return model.AggregateSimulation{Simulations: model.ReverseSimulations(i.sims)}.Cleanup(ctx)
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
		g := obj.GetObjectKind().GroupVersionKind()
		resp[g] = append(resp[g], obj)
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

type createSim struct {
	Spec        kube.Object
	skipCleanup bool
}

var _ model.Simulation = &createSim{}

func newCreateSim(s kube.Object) *createSim {
	return &createSim{Spec: s}
}

func (v *createSim) Run(ctx model.Context) (err error) {
	created, err := ctx.Client.Create(v.Spec)
	if err != nil {
		return err
	}
	if !created {
		v.skipCleanup = true
	}
	return nil
}

func (v *createSim) Cleanup(ctx model.Context) error {
	if v.skipCleanup {
		return nil
	}

	log.Infof("cleaning up %v/%v.%v", v.Spec.GetObjectKind().GroupVersionKind().Kind, v.Spec.GetName(), v.Spec.GetNamespace())
	return ctx.Client.Delete(v.Spec)
}
