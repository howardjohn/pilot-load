package cmd

import (
	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/spf13/cobra"
)

var proberConfig = model.ProberConfig{
	Replicas: 1,
}

func init() {
	proberCmd.PersistentFlags().DurationVar(&proberConfig.Delay, "delay", proberConfig.Delay, "delay between each virtual service")
	proberCmd.PersistentFlags().IntVar(&proberConfig.DelayThreshold, "delay-threshold", proberConfig.DelayThreshold, "if set, there will be no delay until we have this many virtual services")
	proberCmd.PersistentFlags().IntVar(&proberConfig.Replicas, "replicas", proberConfig.Replicas, "number of virtual services to make")
	proberCmd.PersistentFlags().StringVar(&proberConfig.GatewayAddress, "address", proberConfig.GatewayAddress, "address to gateway")
}

var proberCmd = &cobra.Command{
	Use:   "prober",
	Short: "measure the time between resource creation  and it impacting live traffic",
	RunE: func(cmd *cobra.Command, _ []string) error {
		args, err := GetArgs()
		if err != nil {
			return err
		}
		args.ProberConfig = proberConfig
		logConfig(args.ProberConfig)
		return simulation.GatewayProber(args)
	},
}
