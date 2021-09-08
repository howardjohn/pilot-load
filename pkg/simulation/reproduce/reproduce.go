package reproduce

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubescheme "k8s.io/client-go/kubernetes/scheme"

	clientnetworkingalpha "istio.io/client-go/pkg/apis/networking/v1alpha3"
	clientnetworkingbeta "istio.io/client-go/pkg/apis/networking/v1beta1"
	clientsecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	clienttelemetry "istio.io/client-go/pkg/apis/telemetry/v1alpha1"
	"istio.io/istio/pilot/pkg/util/sets"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/test/util/yml"
	"istio.io/pkg/log"
)

type ReproduceSpec struct {
	Delay      time.Duration
	ConfigFile string
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
	toK8s(gvk.Pod),
}

var denylistNamespaces = sets.NewSet("kube-system", "kube-public", "istio-system")

func (i *ReproduceSimulation) Run(ctx model.Context) error {
	cfgsByKind, err := parseInputs(i.Spec.ConfigFile)
	if err != nil {
		return err
	}
	total := 0
	for _, g := range order {
		cfgs := cfgsByKind[g]
		for _, c := range cfgs {
			co := c.DeepCopyObject().(metav1.Object)
			ns := co.GetNamespace()
			name := co.GetName()
			kind := c.GetObjectKind().GroupVersionKind().Kind
			if denylistNamespaces.Contains(ns) {
				continue
			}
			if ns == "default" && name == "kubernetes" {
				continue
			}
			if kind == gvk.Namespace.Kind && denylistNamespaces.Contains(name) {
				continue
			}
			if kind == gvk.Pod.Kind {
				// Getting 500 in API server. Not sure why
				continue
			}

			co.SetResourceVersion("")
			co.SetManagedFields(nil)
			co.SetCreationTimestamp(metav1.Time{})
			co.SetFinalizers(nil)
			if svc, ok := co.(*v1.Service); ok {
				spec := svc.Spec
				if spec.ClusterIP != "None" {
					spec.ClusterIP = ""
					spec.ClusterIPs = nil
				}
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
						subsets[i].Addresses[a].TargetRef = nil
					}
				}
				ep.Subsets = subsets
			}
			s := newCreateSim(co.(runtime.Object))
			i.sims = append(i.sims, s)
			if err := s.Run(ctx); err != nil {
				return err
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
	ybytes, err := ioutil.ReadFile(inputFile)
	if err != nil {
		return nil, err
	}
	codecs := serializer.NewCodecFactory(IstioScheme)
	deserializer := codecs.UniversalDeserializer()

	resp := map[schema.GroupVersionKind][]runtime.Object{}
	for _, chunk := range yml.SplitString(string(ybytes)) {
		var obj runtime.Object
		obj, _, err := deserializer.Decode([]byte(chunk), nil, obj)
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
	Spec        runtime.Object
	skipCleanup bool
}

var _ model.Simulation = &createSim{}

func newCreateSim(s runtime.Object) *createSim {
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
	return ctx.Client.Delete(v.Spec)
}
