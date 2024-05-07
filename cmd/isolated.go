package cmd

import (
	"context"
	"fmt"
	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/isolated"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/monitoring"
	"github.com/howardjohn/pilot-load/pkg/simulation/security"
	"github.com/spf13/cobra"
	kubelib "istio.io/istio/pkg/kube"
	"k8s.io/apimachinery/pkg/watch"
	"net"

	"istio.io/istio/pilot/pkg/features"
	"istio.io/istio/pkg/log"
)

func init() {
	isolatedCmd.PersistentFlags().StringVarP(&configFile, "config", "c", configFile, "config file")
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
		config, err := readConfigFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to read config file: %v", err)
		}
		config = config.ApplyDefaults()
		config.ClusterType = model.Fake

		logConfig(config)
		logClusterConfig(config)
		log.Infof("Starting cluster, total size: %v pods", config.PodCount())

		ms := monitoring.StartMonitoring(8765)
		sim := isolated.NewCluster(isolated.IsolatedSpec{
			Config:         config,
			Fake:           fake,
			Listener:       l,
			MetricsHandler: ms.Handler,
		})
		if err := executeSimulations(args, sim,); err != nil {
			return fmt.Errorf("error executing: %v", err)
		}
		return nil
	},
})

func executeSimulations(a model.Args, s *isolated.Isolated) error {
	// Fork to let us run metrics separately
	ctx, cancel := context.WithCancel(context.Background())
	go simulation.CaptureTermination(ctx, cancel)
	defer cancel()
	simulationContext := model.Context{Context: ctx, Args: a, Client: a.Client, Cancel: cancel}
	if err := s.Run(simulationContext); err != nil {
		log.Errorf("failed: %v, starting cleanup", err)
		cleanupErr := s.Cleanup(simulationContext)
		return fmt.Errorf("failed to run: %v; cleanup: %v", err, cleanupErr)
	}

	<-ctx.Done()
	return s.Cleanup(simulationContext)
}
