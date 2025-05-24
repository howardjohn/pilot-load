package cluster

import (
	"fmt"

	"github.com/howardjohn/pilot-load/pkg/flag"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/spf13/pflag"

	"istio.io/istio/pkg/log"
)

func Command(f *pflag.FlagSet) flag.Command {
	var cfgFile string
	flag.RegisterShort(f, &cfgFile, "config", "c", "config file")
	return flag.Command{
		Name:        "cluster",
		Description: "simulate a full cluster",
		Details:     "",
		Build: func(args *model.Args) (model.DebuggableSimulation, error) {
			config, err := ReadConfigFile(cfgFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read config file: %v", err)
			}
			return Build(args, config), nil
		},
	}
}

func Build(args *model.Args, config Config) *Cluster {
	if len(config.NodeMetadata) > 0 {
		args.Metadata = config.NodeMetadata
	}
	logClusterConfig(config)
	log.Infof("Starting cluster, total size: %v pods", config.PodCount())
	return NewCluster(ClusterSpec{Config: config})
}
