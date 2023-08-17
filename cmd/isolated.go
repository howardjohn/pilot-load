package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime/pprof"

	"github.com/spf13/cobra"
	"istio.io/istio/pilot/pkg/features"
	"istio.io/istio/pilot/pkg/xds"
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/test"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/isolated"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/monitoring"
	"github.com/howardjohn/pilot-load/pkg/simulation/security"
)

func init() {
	isolatedCmd.PersistentFlags().StringVarP(&configFile, "config", "c", configFile, "config file")
}

func WithProfiling(c *cobra.Command) *cobra.Command {
	var cpuProfile string
	var memProfile string
	c.PersistentFlags().StringVar(&cpuProfile, "cpuprofile", cpuProfile, "file to write cpu profile to")
	c.PersistentFlags().StringVar(&memProfile, "memprofile", memProfile, "file to write memory profile to")
	orig := c.RunE
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if cpuProfile != "" {
			f, err := os.Create(cpuProfile)
			if err != nil {
				return err
			}
			if err := pprof.StartCPUProfile(f); err != nil {
				return err
			}
			defer pprof.StopCPUProfile()
		}
		if memProfile != "" {
			f, err := os.Create(memProfile)
			if err != nil {
				return err
			}
			defer func() {
				pprof.WriteHeapProfile(f)
			}()
		}
		return orig(cmd, args)
	}
	return c
}

func executeSimulations(a model.Args, s model.Simulation) error {
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

var isolatedCmd = WithProfiling(&cobra.Command{
	Use:   "isolated",
	Short: "simulate a full cluster in a single binary",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var ds *xds.FakeDiscoveryServer
		ready := make(chan struct{})
		done := make(chan struct{})
		defer func() {
			close(done)
		}()
		// Bump up QPS of requests so test starts faster
		features.RequestLimit = 200.0
		// Kube fake explodes too early
		watch.DefaultChanSize = 10_000
		go test.Wrap(func(t test.Failer) {
			ds = xds.NewFakeDiscoveryServer(t, xds.FakeOptions{
				ListenerBuilder: func() (net.Listener, error) {
					return net.Listen("tcp", "127.0.0.1:0")
				},
			})
			close(ready)
			<-done
		})
		<-ready
		ms := monitoring.StartMonitoring(8765)
		ds.Discovery.InitDebug(ms.Handler.(*http.ServeMux), false, func() map[string]string {
			return nil
		})

		args := model.Args{
			PilotAddress: ds.Listener.Addr().String(),
			Client:       kube.NewFakeClient(ds.KubeClient()),
			Auth: &security.AuthOptions{
				Type: security.AuthTypePlaintext,
			},
		}
		config, err := readConfigFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to read config file: %v", err)
		}
		config = config.ApplyDefaults()

		logConfig(config)
		logClusterConfig(config)
		log.Infof("Starting cluster, total size: %v pods", config.PodCount())

		sim := isolated.NewCluster(isolated.IsolatedSpec{Config: config})
		if err := executeSimulations(args, sim); err != nil {
			return fmt.Errorf("error executing: %v", err)
		}

		return nil
	},
})
