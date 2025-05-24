package cluster

import (
	"fmt"
	"os"
	"text/template"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/templates"
	"sigs.k8s.io/yaml"

	"istio.io/istio/pkg/log"
)

// Cluster defines one single cluster. There is likely only one of these, unless we support multicluster
// A cluster consists of various namespaces
type Config struct {
	// Time between each namespace creation at startup
	GracePeriod model.Duration    `json:"gracePeriod,omitempty"`
	Jitter      JitterConfig      `json:"jitter,omitempty"`
	Namespaces  []NamespaceConfig `json:"namespaces,omitempty"`
	Nodes       []NodeConfig      `json:"nodes,omitempty"`
	// If true, consistent names will be used across iterations.
	StableNames  bool                      `json:"stableNames,omitempty"`
	NodeMetadata map[string]string         `json:"nodeMetadata,omitempty"`
	Templates    model.TemplateDefinitions `json:"templates,omitempty"`
}

type NamespaceConfig struct {
	Name         string                 `json:"name,omitempty"`
	Replicas     int                    `json:"replicas,omitempty"`
	Applications []ApplicationConfig    `json:"applications,omitempty"`
	Templates    []model.ConfigTemplate `json:"configs,omitempty"`
	Waypoint     string                 `json:"waypoint,omitempty"`
}

type ApplicationConfig struct {
	Name      string                 `json:"name,omitempty"`
	Type      model.AppType          `json:"type,omitempty"`
	Replicas  int                    `json:"replicas,omitempty"`
	Pods      int                    `json:"pods,omitempty"`
	Labels    map[string]string      `json:"labels,omitempty"`
	Templates []model.ConfigTemplate `json:"configs,omitempty"`
	GetNode   func() string          `json:"-"`
}

type JitterConfig struct {
	Workloads model.Duration `json:"workloads,omitempty"`
	Config    model.Duration `json:"config,omitempty"`
}

type NodeConfig struct {
	Name    string             `json:"name,omitempty"`
	Ztunnel *NodeZtunnelConfig `json:"ztunnel,omitempty"`
	Count   int                `json:"count,omitempty"`
}

type NodeZtunnelConfig struct{}

func (c Config) ApplyDefaults() Config {
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
				dp.Type = model.PlainType
			}
			ns.Applications[d] = dp
		}
		ret.Namespaces[n] = ns
	}
	return *ret
}

func (c Config) PodCount() int {
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

func (c Config) NodeCount() int {
	cnt := 0
	for _, n := range c.Nodes {
		cnt += n.Count
	}
	return cnt
}

var defaultConfig = Config{
	Namespaces: []NamespaceConfig{{
		Applications: []ApplicationConfig{{Pods: 1}},
	}},
}

func ReadConfigFile(filename string) (Config, error) {
	if filename == "" {
		return defaultConfig, nil
	}
	bytes, err := os.ReadFile(filename)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read configFile file: %v", filename)
	}
	return ReadConfig(string(bytes))
}

func ReadConfig(cfgBytes string) (Config, error) {
	config := Config{}
	if err := yaml.Unmarshal([]byte(cfgBytes), &config); err != nil {
		return config, fmt.Errorf("failed to unmarshall configFile: %v", err)
	}
	if config.Templates.Inner == nil {
		config.Templates.Inner = map[string]*template.Template{}
	}
	for k, v := range templates.LoadBuiltin() {
		if _, f := config.Templates.Inner[k]; f {
			log.Warnf("warning: overriding default template %q", k)
			continue
		}
		config.Templates.Inner[k] = v
	}
	return config.ApplyDefaults(), nil
}

func logClusterConfig(config Config) {
	namespaces, pods, applications := 0, 0, 0
	for _, ns := range config.Namespaces {
		namespaces += ns.Replicas
		for _, app := range ns.Applications {
			applications += app.Replicas * ns.Replicas
			pods += app.Replicas * app.Pods * ns.Replicas
		}
	}
	log.Infof("Initial configuration: %d namespaces, %d applications, and %d pods", namespaces, applications, pods)
}
