package cmd

import (
	"github.com/spf13/cobra"

	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

var startupConfig = model.StartupConfig{
	InCluster:   false,
	Concurrency: 1,
}

func init() {
	startupCmd.PersistentFlags().BoolVar(&startupConfig.InCluster, "incluster", startupConfig.InCluster, "whether we are running in cluster. If enabled, we will check the readiness probe.")
	startupCmd.PersistentFlags().IntVar(&startupConfig.Concurrency, "concurrency", startupConfig.Concurrency, "number of pods to start concurrently")
	startupCmd.PersistentFlags().StringVar(&startupConfig.Namespace, "namespace", startupConfig.Namespace, "namespace to run in")
}

var startupCmd = &cobra.Command{
	Use:   "startup",
	Short: "measure the time for pods to start",
	RunE: func(cmd *cobra.Command, _ []string) error {
		args, err := GetArgs()
		if err != nil {
			return err
		}
		args.StartupConfig = startupConfig
		logConfig(args.StartupConfig)
		return simulation.PodStartup(args)
	},
}
