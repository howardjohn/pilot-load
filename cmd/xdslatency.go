package cmd

import (
	"github.com/spf13/cobra"

	"github.com/howardjohn/pilot-load/pkg/simulation"
)

var xdsLatencyCmd = &cobra.Command{
	Use:   "latency",
	Short: "measure end to end XDS latency",
	RunE: func(cmd *cobra.Command, _ []string) error {
		args, err := GetArgs()
		if err != nil {
			return err
		}
		return simulation.Latency(args)
	},
}
