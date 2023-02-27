package cmd

import (
	"fmt"
	"io/ioutil"

	"github.com/ghodss/yaml"
	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/spf13/cobra"

	"istio.io/pkg/log"
)

var (
	configFile  = ""
	realCluster = false
)

func init() {
	clusterCmd.PersistentFlags().StringVarP(&configFile, "config", "c", configFile, "config file")
	clusterCmd.PersistentFlags().BoolVar(&realCluster, "real-cluster", realCluster, "set to true if using as real cluster, where we are not able to create fake pods, etc")
}

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "simulate a full cluster",
	RunE: func(cmd *cobra.Command, _ []string) error {
		args, err := GetArgs()
		if err != nil {
			return err
		}
		config, err := readConfigFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to read config file: %v", err)
		}
		config = config.ApplyDefaults()
		config.RealCluster = realCluster
		args.ClusterConfig = config
		logConfig(args.ClusterConfig)
		logClusterConfig(args.ClusterConfig)
		return simulation.Cluster(args)
	},
}

var defaultConfig = model.ClusterConfig{
	Namespaces: []model.NamespaceConfig{{
		Applications: []model.ApplicationConfig{{Instances: 1}},
	}},
}

func readConfigFile(filename string) (model.ClusterConfig, error) {
	if filename == "" {
		return defaultConfig, nil
	}
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return model.ClusterConfig{}, fmt.Errorf("failed to read configFile file: %v", filename)
	}
	config := model.ClusterConfig{}
	if err := yaml.Unmarshal(bytes, &config); err != nil {
		return config, fmt.Errorf("failed to unmarshall configFile: %v", err)
	}
	return config, err
}

func logClusterConfig(config model.ClusterConfig) {
	namespaces, pods, applications := 0, 0, 0
	for _, ns := range config.Namespaces {
		namespaces += ns.Replicas
		for _, app := range ns.Applications {
			applications += app.Replicas * ns.Replicas
			pods += app.Replicas * app.Instances * ns.Replicas
		}
	}
	log.Infof("Initial configuration: %d namespaces, %d applications, and %d pods", namespaces, applications, pods)
}

func RunClusterMode() error {
	args, err := GetArgs()
	if err != nil {
		return err
	}
	config, err := readConfigFile("/usr/local/google/home/hemendrat/workspace/pilot-load-hemendrat/pilot-load/example-config.yaml")
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}
	config = config.ApplyDefaults()
	config.RealCluster = realCluster
	args.ClusterConfig = config
	logConfig(args.ClusterConfig)
	logClusterConfig(args.ClusterConfig)
	return simulation.Cluster(args)
}
