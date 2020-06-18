package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/grpclog"
	"istio.io/pkg/log"

	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

var (
	pilotAddress = "localhost:15010"
	metadata     = ""
	kubeconfig   = os.Getenv("KUBECONFIG")
	// TODO scoping, so we can have config dump split from debug
	verbose = false
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&pilotAddress, "pilot-address", "p", pilotAddress, "address to pilot")
	rootCmd.PersistentFlags().StringVarP(&metadata, "metadata", "m", metadata, "metadata to send to pilot")
	rootCmd.PersistentFlags().StringVarP(&kubeconfig, "kubeconfig", "k", kubeconfig, "kubeconfig")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", verbose, "verbose")
}

var rootCmd = &cobra.Command{
	Use:          "pilot-load",
	Short:        "open XDS connections to pilot",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if verbose {
			o := log.DefaultOptions()
			for _, s := range log.Scopes() {
				s.SetOutputLevel(log.DebugLevel)
			}
			o.SetOutputLevel(log.DefaultScopeName, log.DebugLevel)
			if err := log.Configure(o); err != nil {
				panic(err.Error())
			}
		}
		grpclog.SetLoggerV2(grpclog.NewLoggerV2(ioutil.Discard, ioutil.Discard, ioutil.Discard))
		sim := ""
		if len(args) > 0 {
			sim = args[0]
		}
		if kubeconfig == "" {
			kubeconfig = filepath.Join(os.Getenv("HOME"), "/.kube/config")
		}
		a := model.Args{
			PilotAddress: pilotAddress,
			NodeMetadata: metadata,
			KubeConfig:   kubeconfig,
		}
		// TODO read this from config file
		for i := 0; i < 5; i++ {
			a.Cluster.Services = append(a.Cluster.Services, model.WorkloadArgs{Instances: 10})
		}
		switch sim {
		case "cluster":
			return simulation.Cluster(a)
		case "adsc":
			return simulation.Adsc(a)
		default:
			return fmt.Errorf("unknown simulation %v. Expected: {pods, adsc}", sim)
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
