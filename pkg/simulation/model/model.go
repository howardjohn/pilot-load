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

type DeploymentConfig struct {
	Name      string        `json:"name"`
	Replicas  int           `json:"replicas"`
	Instances int           `json:"instances"`
	GetNode   func() string `json:"-"`
}

type NamespaceConfig struct {
	Name        string             `json:"name"`
	Replicas    int                `json:"replicas"`
	Deployments []DeploymentConfig `json:"deployments"`
}

// Cluster defines one single cluster. There is likely only one of these, unless we support multicluster
// A cluster consists of various namespaces
type ClusterConfig struct {
	Jitter       ClusterJitterConfig    `json:"jitter"`
	Namespaces   []NamespaceConfig      `json:"namespaces"`
	Nodes        int                    `json:"nodes"`
	NodeMetadata map[string]interface{} `json:"nodeMetadata"`
}

type Args struct {
	PilotAddress  string
	KubeConfig    string
	ClusterConfig ClusterConfig
}

type Context struct {
	// Overall context. This should not be used to manage cleanup
	context.Context
	Args   Args
	Client *kube.Client
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
