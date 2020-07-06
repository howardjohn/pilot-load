package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"istio.io/pkg/log"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
)

type Simulation interface {
	// Run starts the simulation. If the simulation is long lived, this should be done asynchronously,
	// watching ctx.Done() for termination.
	Run(ctx Context) error
	// Cleanup tears down the simulation.
	Cleanup(ctx Context) error
}

type ScalableSimulation interface {
	Scale(ctx Context, delta int) error

	ScaleTo(ctx Context, n int) error
}

type RefreshableSimulation interface {
	// Refresh will make a change to the simulation. This may mean removing and recreating a pod, changing config, etc
	Refresh(ctx Context) error
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
	Workloads Duration `json:"workloads"`
	Config    Duration `json:"config"`
}

type PodType string

const (
	SidecarType PodType = "sidecar"

	GatewayType PodType = "router"

	ExternalType PodType = "external"
)

type ApplicationConfig struct {
	Name      string        `json:"name"`
	PodType   PodType       `json:"podType"`
	Replicas  int           `json:"replicas"`
	Instances int           `json:"instances"`
	GetNode   func() string `json:"-"`
}

type NamespaceConfig struct {
	Name         string              `json:"name"`
	Replicas     int                 `json:"replicas"`
	Applications []ApplicationConfig `json:"applications"`
}

// Cluster defines one single cluster. There is likely only one of these, unless we support multicluster
// A cluster consists of various namespaces
type ClusterConfig struct {
	Jitter       ClusterJitterConfig    `json:"jitter"`
	Namespaces   []NamespaceConfig      `json:"namespaces"`
	Nodes        int                    `json:"nodes"`
	NodeMetadata map[string]interface{} `json:"nodeMetadata"`
}

func (c ClusterConfig) ApplyDefaults() ClusterConfig {
	cpy := c
	ret := &cpy
	if ret.Nodes == 0 {
		ret.Nodes = 1
	}
	for n, ns := range ret.Namespaces {
		if ns.Replicas == 0 {
			ns.Replicas = 1
		}
		for d, dp := range ns.Applications {
			if dp.Replicas == 0 {
				dp.Replicas = 1
			}
			if dp.PodType == "" {
				dp.PodType = SidecarType
			}
			ns.Applications[d] = dp
		}
		ret.Namespaces[n] = ns
	}
	return *ret
}

type AdscConfig struct {
	Count int
}

type Args struct {
	PilotAddress  string
	InjectAddress string
	KubeConfig    string
	Qps           int
	ClusterConfig ClusterConfig
	AdsConfig     AdscConfig
}

type Context struct {
	// Overall context. This should not be used to manage cleanup
	context.Context
	Args   Args
	Client *kube.Client
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
}

var _ Simulation = AggregateSimulation{}

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
	}
	return nil
}

// TODO parallelize?
func (a AggregateSimulation) Cleanup(ctx Context) error {
	var err error
	for _, s := range a.Simulations {
		log.Debugf("cleaning simulation %T", s)
		if err := s.Cleanup(ctx); err != nil {
			err = util.AddError(err, fmt.Errorf("failed cleaning simulation %T: %v", s, err))
		}
	}
	return err
}
