package cmd

import (
	"github.com/spf13/cobra"

	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

var impersonateConfig = model.ImpersonateConfig{
	Replicas: 1,
	Selector: string(model.SidecarSelector),
}

func init() {
	impersonateCmd.PersistentFlags().DurationVar(&impersonateConfig.Delay, "delay", impersonateConfig.Delay, "delay between each connection")
	impersonateCmd.PersistentFlags().IntVar(&impersonateConfig.Replicas, "replicas", impersonateConfig.Replicas, "number of connections to make for each pod")
	impersonateCmd.PersistentFlags().StringVar(&impersonateConfig.Selector, "selector", impersonateConfig.Selector, "selector to use {sidecar,external,both}")
	impersonateCmd.PersistentFlags().BoolVar(&impersonateConfig.Watch, "watch", impersonateConfig.Watch, "watch for pod changes")
}

var impersonateCmd = &cobra.Command{
	Use:   "impersonate",
	Short: "simulate ADS connections from real pods running in the cluster",
	RunE: func(cmd *cobra.Command, _ []string) error {
		args, err := GetArgs()
		if err != nil {
			return err
		}
		args.ImpersonateConfig = impersonateConfig
		logConfig(args.ImpersonateConfig)
		return simulation.Impersonate(args)
	},
}
