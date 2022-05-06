package cmd

import (
	"fmt"
	"time"

	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/spf13/cobra"
)

var startupConfig = model.StartupConfig{
	InCluster:   false,
	Concurrency: 1,
	Cooldown:    time.Millisecond * 10,
}

func init() {
	startupCmd.PersistentFlags().BoolVar(&startupConfig.InCluster, "incluster", startupConfig.InCluster, "whether we are running in cluster. If enabled, we will check the readiness probe.")
	startupCmd.PersistentFlags().IntVar(&startupConfig.Concurrency, "concurrency", startupConfig.Concurrency, "number of pods to start concurrently")
	startupCmd.PersistentFlags().StringVar(&startupConfig.Namespace, "namespace", startupConfig.Namespace, "namespace to run in")
	startupCmd.PersistentFlags().DurationVar(&startupConfig.Cooldown, "cooldown", startupConfig.Cooldown, "time to wait after starting each pod (per worker)")
}

var startupCmd = &cobra.Command{
	Use:   "startup",
	Short: "measure the time for pods to start",
	RunE: func(cmd *cobra.Command, _ []string) error {
		args, err := GetArgs()
		if err != nil {
			return err
		}
		if startupConfig.Namespace == "" {
			return fmt.Errorf("--namespace required")
		}
		args.StartupConfig = startupConfig
		logConfig(args.StartupConfig)
		return simulation.PodStartup(args)
	},
}
