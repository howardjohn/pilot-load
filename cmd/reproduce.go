package cmd

import (
	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/spf13/cobra"
)

var reproduceConfig = model.ReproduceConfig{}

func init() {
}

func init() {
	reproduceCmd.PersistentFlags().DurationVar(&reproduceConfig.Delay, "delay", reproduceConfig.Delay, "delay between each connection")
	reproduceCmd.PersistentFlags().StringVarP(&reproduceConfig.ConfigFile, "file", "f", reproduceConfig.ConfigFile, "config file")
}

var reproduceCmd = &cobra.Command{
	Use:   "reproduce",
	Short: "simulate ADS connections from input YAML",
	RunE: func(cmd *cobra.Command, _ []string) error {
		args, err := GetArgs()
		if err != nil {
			return err
		}
		args.ReproduceConfig = reproduceConfig
		logConfig(args.ReproduceConfig)
		return simulation.Reproduce(args)
	},
}
