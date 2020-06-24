package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/grpclog"
	"istio.io/pkg/log"

	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

var (
	pilotAddress   = "localhost:15010"
	kubeconfig     = os.Getenv("KUBECONFIG")
	configFile     = ""
	loggingOptions = defaultLogOptions()
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&pilotAddress, "pilot-address", "p", pilotAddress, "address to pilot")
	rootCmd.PersistentFlags().StringVarP(&kubeconfig, "kubeconfig", "k", kubeconfig, "kubeconfig")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", configFile, "config file")
}

func defaultLogOptions() *log.Options {
	o := log.DefaultOptions()

	// These scopes are, at the default "INFO" level, too chatty for command line use
	o.SetOutputLevel("dump", log.WarnLevel)

	return o
}

var rootCmd = &cobra.Command{
	Use:          "pilot-load",
	Short:        "open XDS connections to pilot",
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return log.Configure(loggingOptions)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		grpclog.SetLoggerV2(grpclog.NewLoggerV2(ioutil.Discard, ioutil.Discard, ioutil.Discard))
		sim := ""
		if len(args) > 0 {
			sim = args[0]
		}
		if kubeconfig == "" {
			kubeconfig = filepath.Join(os.Getenv("HOME"), "/.kube/configFile")
		}
		config, err := readConfigFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to read config file: %v", err)
		}
		bytes, err := yaml.Marshal(config)
		if err != nil {
			return err
		}
		log.Infof("Starting simulation with config:\n%v", string(bytes))
		a := model.Args{
			PilotAddress:  pilotAddress,
			KubeConfig:    kubeconfig,
			ClusterConfig: config,
		}

		switch sim {
		case "cluster":
			return simulation.Cluster(a)
		case "adsc":
			return simulation.Adsc(a)
		default:
			return fmt.Errorf("unknown simulation %v. Expected: {cluster, adsc}", sim)
		}
	},
}

var defaultConfig = model.ClusterConfig{
	Jitter: model.ClusterJitterConfig{
		Workloads: 0,
		Config:    0,
	},
	Namespaces: []model.NamespaceConfig{{
		Name:     "default",
		Replicas: 1,
		Deployments: []model.DeploymentConfig{{
			Name:      "default",
			Replicas:  1,
			Instances: 1,
		}},
	}},
}

func readConfigFile(filename string) (model.ClusterConfig, error) {
	if filename == "" {
		return defaultConfig, nil
	}
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return model.ClusterConfig{}, fmt.Errorf("failed to read configFile file: %v", filename)
	}
	config := model.ClusterConfig{}
	if err := yaml.Unmarshal(bytes, &config); err != nil {
		return config, fmt.Errorf("failed to unmarshall configFile: %v", err)
	}
	return config, err
}

func Execute() {
	loggingOptions.AttachCobraFlags(rootCmd)
	hiddenFlags := []string{"log_as_json", "log_rotate", "log_rotate_max_age", "log_rotate_max_backups",
		"log_rotate_max_size", "log_stacktrace_level", "log_target", "log_caller"}
	for _, opt := range hiddenFlags {
		_ = rootCmd.PersistentFlags().MarkHidden(opt)
	}
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
