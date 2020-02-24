package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/howardjohn/pilot-load/pkg/simulation"
)

var (
	pilotAddress = "localhost:15010"
	metadata     = ""
	ipaddress    = "128.0.0.1"
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&pilotAddress, "pilot-address", "p", pilotAddress, "address to pilot")
	rootCmd.PersistentFlags().StringVarP(&metadata, "metadata", "m", metadata, "metadata to send to pilot")
	rootCmd.PersistentFlags().StringVarP(&ipaddress, "ipaddress", "i", ipaddress, "ipaddress to use to connect to pilot")
}

var rootCmd = &cobra.Command{
	Use:          "pilot-load",
	Short:        "open XDS connections to pilot",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sim := ""
		if len(args) > 0 {
			sim = args[0]
		}
		a := simulation.Args{
			PilotAddress: pilotAddress,
			NodeMetadata: metadata,
		}
		switch sim {
		case "adsc":
			return simulation.Adsc(a, ipaddress)

		default:
			return simulation.Simple(a)
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
