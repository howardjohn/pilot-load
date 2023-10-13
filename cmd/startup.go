package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

var startupConfig = model.StartupConfig{
	Concurrency: 1,
	Cooldown:    time.Millisecond * 10,
}
var specFile string

func init() {
	startupCmd.PersistentFlags().BoolVar(&startupConfig.Inject, "inject", startupConfig.Inject, "if true, we will inject the pod")
	startupCmd.PersistentFlags().IntVar(&startupConfig.Concurrency, "concurrency", startupConfig.Concurrency, "number of pods to start concurrently")
	startupCmd.PersistentFlags().StringVar(&startupConfig.Namespace, "namespace", startupConfig.Namespace, "namespace to run in")
	startupCmd.PersistentFlags().DurationVar(&startupConfig.Cooldown, "cooldown", startupConfig.Cooldown, "time to wait after starting each pod (per worker)")
	startupCmd.PersistentFlags().StringVar(&specFile, "spec", specFile, "pod spec")
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
		if specFile == "" {
			return fmt.Errorf("--spec required")
		}
		f, err := os.ReadFile(specFile)
		if err != nil {
			return err
		}
		args.StartupConfig.Spec = string(f)

		args.StartupConfig = startupConfig
		logConfig(args.StartupConfig)
		return simulation.PodStartup(args)
	},
}
