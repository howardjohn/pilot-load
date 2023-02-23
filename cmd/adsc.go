package cmd

import (
	"time"

	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/spf13/cobra"
)

var adscConfig = model.AdscConfig{
	Delay:     time.Millisecond * 10,
	Count:     1,
	Namespace: "default",
}

func init() {
	adscCmd.PersistentFlags().DurationVar(&adscConfig.Delay, "delay", adscConfig.Delay, "delay between each connection")
	adscCmd.PersistentFlags().IntVar(&adscConfig.Count, "count", adscConfig.Count, "number of adsc connections to make")
	adscCmd.PersistentFlags().StringVar(&adscConfig.Namespace, "namespace", adscConfig.Namespace, "namespace of simulation")
}

var adscCmd = &cobra.Command{
	Use:   "adsc",
	Short: "open simple ADS connection to Istiod",
	RunE: func(cmd *cobra.Command, _ []string) error {
		args, err := GetArgs()
		if err != nil {
			return err
		}
		args.AdsConfig = adscConfig
		logConfig(args.AdsConfig)
		return simulation.Adsc(args)
	},
}
