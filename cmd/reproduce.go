package cmd

import (
	"github.com/spf13/cobra"

	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

var reproduceConfig = model.ReproduceConfig{}

func init() {
}

func init() {
	reproduceCmd.PersistentFlags().DurationVar(&reproduceConfig.Delay, "delay", reproduceConfig.Delay, "delay between each connection")
	reproduceCmd.PersistentFlags().BoolVarP(&reproduceConfig.ConfigOnly, "config-only", "n", reproduceConfig.ConfigOnly, "only apply config file, do not connect to XDS")
	reproduceCmd.PersistentFlags().StringVarP(&reproduceConfig.ConfigFile, "file", "f", reproduceConfig.ConfigFile, "config file")
}

var reproduceCmd = &cobra.Command{
	Use:   "reproduce",
	Short: "simulate ADS connections from input YAML",
	Long: "simulate ADS connections from input YAML." +
		"Expected format: `kubectl get vs,gw,dr,sidecar,svc,endpoints,pod,namespace,sa -oyaml -A | kubectl grep`",
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
