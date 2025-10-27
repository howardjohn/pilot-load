package flag

import (
	"os"
	"path/filepath"
	"runtime/pprof"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"istio.io/istio/pkg/log"
	"sigs.k8s.io/yaml"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/security"
)

type CommandBuilder = func(f *pflag.FlagSet) Command

type Command struct {
	Name        string
	Description string
	Details     string
	Build       func(args *model.Args) (model.DebuggableSimulation, error)
}

func GetArgs() (model.Args, error) {
	var err error
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), "/.kube/config")
	}
	cl, err := kube.NewClient(kubeconfig, qps)
	if err != nil {
		return model.Args{}, err
	}
	auth := security.AuthType(auth)
	if auth == "" {
		auth = security.DefaultAuthForAddress(pilotAddress)
	}
	authOpts := &security.AuthOptions{
		Type:   auth,
		Client: cl,
	}
	args := model.Args{
		PilotAddress: pilotAddress,
		DeltaXDS:     delta,
		Metadata:     xdsMetadata,
		Client:       cl,
		Auth:         authOpts,
	}
	return args, nil
}

func BuildCobra(cb CommandBuilder) *cobra.Command {
	cmd := &cobra.Command{}
	fs := cmd.Flags()
	built := cb(fs)
	cmd.Use = built.Name
	cmd.Short = built.Description
	cmd.Long = built.Description + "\n" + built.Details
	cmd.SilenceUsage = true
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return log.Configure(loggingOptions)
	}
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		args, err := GetArgs()
		if err != nil {
			return err
		}
		sim, err := built.Build(&args)
		if err != nil {
			return err
		}
		logConfig(sim.GetConfig())
		return simulation.ExecuteSimulations(args, sim)
	}
	return cmd
}

func logConfig(config interface{}) {
	bytes, err := yaml.Marshal(config)
	if err != nil {
		panic(err.Error())
	}
	log.Infof("Starting simulation with config:\n%v", string(bytes))
}

func RunMain(command func(f *pflag.FlagSet) Command) {
	c := WithProfiling(BuildCobra(command))
	AttachGlobalFlags(c)
	if err := c.Execute(); err != nil {
		os.Exit(1)
	}
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
				_ = pprof.WriteHeapProfile(f)
			}()
		}
		return orig(cmd, args)
	}
	return c
}
