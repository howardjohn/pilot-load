package cmd

import (
	"context"
	"fmt"
	"net"

	"github.com/spf13/cobra"
	"istio.io/istio/pilot/pkg/features"
	kubelib "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/log"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/isolated"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/monitoring"
	"github.com/howardjohn/pilot-load/pkg/simulation/security"
)

var (
	shortCircuit  = false
	reproduceFile = ""
)

func init() {
	isolatedCmd.PersistentFlags().StringVarP(&configFile, "config", "c", configFile, "config file, for simulation")
	isolatedCmd.PersistentFlags().BoolVarP(&shortCircuit, "shortCircuit", "s", shortCircuit, "exit once synced")
	isolatedCmd.PersistentFlags().StringVarP(&reproduceFile, "raw-config", "r", reproduceFile, "config file, for direct Kubernetes YAML")
}

var isolatedCmd = WithProfiling(&cobra.Command{
	Use:   "isolated",
	Short: "simulate a full cluster in a single binary",
	RunE: func(cmd *cobra.Command, _ []string) error {
		// Bump up QPS of requests so test starts faster
		features.RequestLimit = 200.0
		// Kube fake explodes too early
		watch.DefaultChanSize = 100_000
		l, err := net.Listen("tcp", "0.0.0.0:15010")
		if err != nil {
			return err
		}
		fake := kubelib.NewFakeClient()

		args := model.Args{
			PilotAddress: l.Addr().String(),
			Client:       kube.NewFakeClient(fake),
			Auth: &security.AuthOptions{
				Type: security.AuthTypePlaintext,
			},
		}
		ms := monitoring.StartMonitoring(8765)
		isolatedSpec := isolated.IsolatedSpec{
			Fake:           fake,
			Listener:       l,
			MetricsHandler: ms.Handler,
		}
		if configFile != "" {
			config, err := readConfigFile(configFile)
			if err != nil {
				return fmt.Errorf("failed to read config file: %v", err)
			}
			config = config.ApplyDefaults()
			config.ClusterType = model.Fake

			logConfig(config)
			logClusterConfig(config)
			log.Infof("Starting cluster, total size: %v pods", config.PodCount())
			isolatedSpec.ClusterConfig = &config
		} else {
			isolatedSpec.ReproduceConfig = &model.ReproduceConfig{
				ConfigFile: reproduceFile,
			}
		}
		sim := isolated.NewCluster(isolatedSpec)
		if err := executeSimulations(args, sim); err != nil {
			return fmt.Errorf("error executing: %v", err)
		}
		return nil
	},
})

func executeSimulations(a model.Args, s *isolated.Isolated) error {
	// Fork to let us run metrics separately and removing cleanup
	ctx, cancel := context.WithCancel(context.Background())
	go simulation.CaptureTermination(ctx, cancel)
	defer cancel()
	simulationContext := model.Context{Context: ctx, Args: a, Client: a.Client, Cancel: cancel}
	if err := s.Run(simulationContext); err != nil {
		log.Errorf("failed: %v, starting cleanup", err)
		return fmt.Errorf("failed to run: %v", err)
	}

	if !shortCircuit {
		<-ctx.Done()
	}
	return nil
}
