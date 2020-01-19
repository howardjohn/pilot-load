package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/howardjohn/pilot-load/pkg/simulation"
)

var (
	pilotAddress = "localhost:15010"
	prefix       = 128
	clients      = 1
	verbose      = false
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&pilotAddress, "pilot-address", "p", pilotAddress, "address to pilot")
	rootCmd.PersistentFlags().IntVarP(&clients, "clients", "c", clients, "number of clients to connect")
	rootCmd.PersistentFlags().IntVar(&prefix, "prefix", prefix, "IP prefix of the node connecting")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", verbose, "enable adsc logging")
}

var rootCmd = &cobra.Command{
	Use:          "pilot-load",
	Short:        "open XDS connections to pilot",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return simulation.Simple(simulation.Args{
			PilotAddress: pilotAddress,
		})
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
