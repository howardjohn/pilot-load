package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"istio.io/istio/pkg/log"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/yaml"

	"github.com/howardjohn/pilot-load/pkg/flag"
)

var rootCmd = &cobra.Command{
	Use:   "pilot-load",
	Short: "toolkit for commands to load test Istiod and Kubernetes",
}

func logConfig(config interface{}) {
	bytes, err := yaml.Marshal(config)
	if err != nil {
		panic(err.Error())
	}
	log.Infof("Starting simulation with config:\n%v", string(bytes))
}

func init() {
	rootCmd.AddCommand(
		adscCmd,
		dumpCmd,
	)
	flag.AttachGlobalFlags(rootCmd)
	for _, cb := range commands {
		cmd := flag.BuildCobra(cb)
		rootCmd.AddCommand(cmd)
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
