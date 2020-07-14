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
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

var (
	pilotAddress   = "localhost:15010"
	kubeconfig     = os.Getenv("KUBECONFIG")
	configFile     = ""
	loggingOptions = defaultLogOptions()
	adscConfig     = model.AdscConfig{}
	qps            = 100
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&pilotAddress, "pilot-address", "p", pilotAddress, "address to pilot")
	rootCmd.PersistentFlags().StringVarP(&kubeconfig, "kubeconfig", "k", kubeconfig, "kubeconfig")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", configFile, "config file")
	rootCmd.PersistentFlags().IntVar(&qps, "qps", qps, "qps for kube client")

	rootCmd.PersistentFlags().IntVar(&adscConfig.Count, "adsc.count", adscConfig.Count, "number of adsc connections to make")
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
			kubeconfig = filepath.Join(os.Getenv("HOME"), "/.kube/config")
		}
		config, err := readConfigFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to read config file: %v", err)
		}
		config = config.ApplyDefaults()
		logConfig(config)
		if qps == 0 {
			qps = 100
		}
		a := model.Args{
			PilotAddress:  pilotAddress,
			KubeConfig:    kubeconfig,
			Qps:           qps,
			ClusterConfig: config,
			AdsConfig:     adscConfig,
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

func logConfig(config model.ClusterConfig) {
	bytes, err := yaml.Marshal(config)
	if err != nil {
		panic(err.Error())
	}
	log.Infof("Starting simulation with config:\n%v", string(bytes))
	namespaces, pods, applications := 0, 0, 0
	for _, ns := range config.Namespaces {
		namespaces += ns.Replicas
		for _, app := range ns.Applications {
			applications += app.Replicas
			pods += app.Replicas * app.Instances
		}
	}
	log.Infof("Initial configuration: %d namespaces, %d applications, and %d pods", namespaces, applications, pods)
}

var defaultConfig = model.ClusterConfig{
	Namespaces: []model.NamespaceConfig{{
		Applications: []model.ApplicationConfig{{Instances: 1}},
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
