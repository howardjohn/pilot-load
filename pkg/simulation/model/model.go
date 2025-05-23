package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"text/template"
	"time"

	"github.com/Masterminds/sprig"
	"golang.org/x/sync/errgroup"
	"istio.io/istio/pkg/log"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/security"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
)

type Simulation interface {
	// Run starts the simulation. If the simulation is long lived, this should be done asynchronously,
	// watching ctx.Done() for termination.
	Run(ctx Context) error
	// Cleanup tears down the simulation.
	Cleanup(ctx Context) error
}

type DebuggableSimulation interface {
	Simulation
	// GetConfig returns the config for the simulation
	GetConfig() any
}

type RunningSimulation interface {
	Simulation
	Running() chan struct{}
}

type ScalableSimulation interface {
	Scale(ctx Context, delta int) error
	ScaleTo(ctx Context, n int) error
}

type RefreshableSimulation interface {
	// Refresh will make a change to the simulation. This may mean removing and recreating a pod, changing config, etc
	// The returned string gives info about what was refreshed
	Refresh(ctx Context) (string, error)
}

func IsRefreshable(s RefreshableSimulation) bool {
	if t, ok := s.(MaybeRefreshableSimulation); ok {
		return t.IsRefreshable()
	}
	return true
}

type MaybeRefreshableSimulation interface {
	// IsRefreshable indicates whether we can refresh the config
	IsRefreshable() bool
}

type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		*d = Duration(time.Duration(value))
		return nil
	case string:
		tmp, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		*d = Duration(tmp)
		return nil
	default:
		return errors.New("invalid duration")
	}
}

type ClusterJitterConfig struct {
	Workloads Duration `json:"workloads,omitempty"`
	Config    Duration `json:"config,omitempty"`
}

type AppType string

type APIScope string

func (p AppType) HasProxy() bool {
	return p == SidecarType || p == GatewayType || p == WaypointType
}

const (
	PlainType    AppType = "plain"
	SidecarType  AppType = "sidecar"
	AmbientType  AppType = "ambient"
	WaypointType AppType = "waypoint"
	GatewayType  AppType = "gateway"
	ExternalType AppType = "external"
	ZtunnelType  AppType = "ztunnel"
	VMType       AppType = "vm"
)

type ApplicationConfig struct {
	Name      string            `json:"name,omitempty"`
	Type      AppType           `json:"type,omitempty"`
	Replicas  int               `json:"replicas,omitempty"`
	Pods      int               `json:"pods,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Templates []ConfigTemplate  `json:"configs,omitempty"`
	GetNode   func() string     `json:"-"`
}

type ConfigTemplate struct {
	Name    string         `json:"name,omitempty"`
	Config  map[string]any `json:"config,omitempty"`
	Refresh *bool          `json:"refresh,omitempty"`
}

func (r *ConfigTemplate) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal as a string
	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		// If it worked, set the name and return
		r.Name = stringValue
		return nil
	}

	// If that didn't work, try to unmarshal as an object
	type ConfigTemplateAlias ConfigTemplate // Create alias to avoid infinite recursion
	return json.Unmarshal(data, (*ConfigTemplateAlias)(r))
}

type NamespaceConfig struct {
	Name         string              `json:"name,omitempty"`
	Replicas     int                 `json:"replicas,omitempty"`
	Applications []ApplicationConfig `json:"applications,omitempty"`
	Templates    []ConfigTemplate    `json:"configs,omitempty"`
	Waypoint     string              `json:"waypoint,omitempty"`
}

// Cluster defines one single cluster. There is likely only one of these, unless we support multicluster
// A cluster consists of various namespaces
type ClusterConfig struct {
	// Time between each namespace creation at startup
	GracePeriod Duration            `json:"gracePeriod,omitempty"`
	Jitter      ClusterJitterConfig `json:"jitter,omitempty"`
	Namespaces  []NamespaceConfig   `json:"namespaces,omitempty"`
	Nodes       []NodeConfig        `json:"nodes,omitempty"`
	// If true, consistent names will be used across iterations.
	StableNames  bool                `json:"stableNames,omitempty"`
	NodeMetadata map[string]string   `json:"nodeMetadata,omitempty"`
	Templates    TemplateDefinitions `json:"templates,omitempty"`
}

type TemplateDefinitions struct {
	Inner map[string]*template.Template `json:"-"`
}

func (t *TemplateDefinitions) UnmarshalJSON(b []byte) error {
	raw := make(map[string]string)
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	t.Inner = make(map[string]*template.Template)
	for k, v := range raw {
		p, err := template.New(k).Funcs(sprig.TxtFuncMap()).Parse(v)
		if err != nil {
			return err
		}
		t.Inner[k] = p
	}
	return nil
}

func (t *TemplateDefinitions) Get(name string) *template.Template {
	tt, ok := t.Inner[name]
	if !ok {
		panic("unknown template name: " + name)
	}
	return tt
}

type NodeConfig struct {
	Name    string             `json:"name,omitempty"`
	Ztunnel *NodeZtunnelConfig `json:"ztunnel,omitempty"`
	Count   int                `json:"count,omitempty"`
}

type NodeZtunnelConfig struct{}

func (c ClusterConfig) ApplyDefaults() ClusterConfig {
	cpy := c
	ret := &cpy
	if len(ret.Nodes) == 0 {
		ret.Nodes = []NodeConfig{{Count: 1, Name: "default"}}
	}
	for n, ns := range ret.Namespaces {
		if ns.Replicas == 0 {
			ns.Replicas = 1
		}
		for d, dp := range ns.Applications {
			if dp.Replicas == 0 {
				dp.Replicas = 1
			}
			if dp.Type == "" {
				dp.Type = PlainType
			}
			ns.Applications[d] = dp
		}
		ret.Namespaces[n] = ns
	}
	return *ret
}

func (c ClusterConfig) PodCount() int {
	cnt := 0
	for _, ns := range c.Namespaces {
		apps := 0
		for _, app := range ns.Applications {
			apps += app.Replicas * app.Pods
		}
		cnt += apps * ns.Replicas
	}
	return cnt
}

func (c ClusterConfig) NodeCount() int {
	cnt := 0
	for _, n := range c.Nodes {
		cnt += n.Count
	}
	return cnt
}

type DumpConfig struct {
	Pod       string
	Namespace string
	OutputDir string
}

type AdscConfig struct {
	Count     int
	Delay     time.Duration
	Namespace string
}

type Selector string

var (
	SidecarSelector  Selector = "sidecar"
	ExternalSelector Selector = "external"
	BothSelector     Selector = "both"
)

type ImpersonateConfig struct {
	Replicas int
	Delay    time.Duration
	Selector string
}

type ReproduceConfig struct {
	ConfigFile string
	ConfigOnly bool
	Delay      time.Duration
}

type StartupConfig struct {
	Namespace   string
	Concurrency int
	Inject      bool
	Cooldown    time.Duration
	Spec        string
}

type Args struct {
	PilotAddress  string
	InjectAddress string
	Client        *kube.Client
	Auth          *security.AuthOptions
	ClusterConfig ClusterConfig
	AdsConfig     AdscConfig
	Metadata      map[string]string
	DeltaXDS      bool
	DumpConfig    DumpConfig
}

type Context struct {
	// Overall context. This should not be used to manage cleanup
	context.Context
	Args   Args
	Client *kube.Client
	Cancel context.CancelFunc
}

func ReverseSimulations(sims []Simulation) []Simulation {
	for i := 0; i < len(sims)/2; i++ {
		j := len(sims) - i - 1
		sims[i], sims[j] = sims[j], sims[i]
	}
	return sims
}

type AggregateSimulation struct {
	Simulations []Simulation
	Delay       time.Duration
}

var _ Simulation = AggregateSimulation{}

func (a AggregateSimulation) RunParallel(ctx Context) error {
	g := errgroup.Group{}
	for _, s := range a.Simulations {
		log.Debugf("running simulation in parallel %T", s)
		g.Go(func() error {
			if err := s.Run(ctx); err != nil {
				return fmt.Errorf("failed running simulation %T: %v", s, err)
			}
			return nil
		})
		util.ContextSleep(ctx, a.Delay)
	}
	return g.Wait()
}

func (a AggregateSimulation) Run(ctx Context) error {
	for _, s := range a.Simulations {
		if util.IsDone(ctx) {
			log.Warnf("exiting early; context cancelled")
			return nil
		}
		log.Debugf("running simulation %T", s)
		if err := s.Run(ctx); err != nil {
			return fmt.Errorf("failed running simulation %T: %v", s, err)
		}
		util.ContextSleep(ctx, a.Delay)
	}
	return nil
}

func (a AggregateSimulation) CleanupParallel(ctx Context) error {
	g := errgroup.Group{}
	g.SetLimit(100)
	for _, s := range a.Simulations {
		log.Debugf("cleaning simulation %T", s)
		g.Go(func() error {
			if err := s.Cleanup(ctx); err != nil {
				return fmt.Errorf("failed cleaning simulation %T: %v", s, err)
			}
			return nil
		})
	}
	return g.Wait()
}

func (a AggregateSimulation) Cleanup(ctx Context) error {
	var errs error
	for _, s := range a.Simulations {
		log.Debugf("cleaning simulation %T", s)
		if err := s.Cleanup(ctx); err != nil {
			errs = util.AddError(errs, fmt.Errorf("failed cleaning simulation %T: %v", s, err))
		}
	}
	return errs
}
