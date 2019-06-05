package cmd

import (
	"fmt"
	"github.com/howardjohn/pilot-load/client"
	"github.com/spf13/cobra"
	"os"
)

var (
	pilotAddress = "localhost:15010"
	clients      = 1
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&pilotAddress, "pilot-address", "p", pilotAddress, "address to pilot")
	rootCmd.PersistentFlags().IntVarP(&clients, "clients", "c", clients, "number of clients to connect")
}

var rootCmd = &cobra.Command{
	Use:   "pilot-load",
	Short: "open XDS connections to pilot",
	RunE: func(cmd *cobra.Command, args []string) error {
		return client.RunLoad(pilotAddress, clients)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
