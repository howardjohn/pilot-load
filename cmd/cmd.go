package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	"istio.io/istio/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/security"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
)

var (
	pilotAddress   = defaultAddress()
	xdsMetadata    = map[string]string{}
	auth           = string(security.AuthTypeDefault)
	delta          = false
	kubeconfig     = os.Getenv("KUBECONFIG")
	loggingOptions = defaultLogOptions()

	authTrustDomain   = ""
	authClusterUrl    = ""
	authProjectNumber = ""

	qps = 10000
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&pilotAddress, "pilot-address", "p", pilotAddress, "address to pilot")
	rootCmd.PersistentFlags().StringVarP(&auth, "auth", "a", auth,
		fmt.Sprintf("auth type use. If not set, default based on port number. Supported options: %v", security.AuthTypeOptions()))
	rootCmd.PersistentFlags().StringVarP(&kubeconfig, "kubeconfig", "k", kubeconfig, "kubeconfig")
	rootCmd.PersistentFlags().IntVar(&qps, "qps", qps, "qps for kube client")
	rootCmd.PersistentFlags().StringToStringVarP(&xdsMetadata, "metadata", "m", xdsMetadata, "xds metadata")

	rootCmd.PersistentFlags().BoolVar(&delta, "delta", delta, "use delta XDS")

	rootCmd.PersistentFlags().StringVar(&authClusterUrl, "clusterURL", authClusterUrl, "cluster URL (for google auth)")
	rootCmd.PersistentFlags().StringVar(&authTrustDomain, "trustDomain", authTrustDomain, "trust domain (for google auth)")
	rootCmd.PersistentFlags().StringVar(&authProjectNumber, "projectNumber", authProjectNumber, "project number (for google auth)")
}

func defaultAddress() string {
	_, inCluster := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	if inCluster {
		return "istiod.istio-system.svc:15010"
	}
	return "localhost:15010"
}

func defaultLogOptions() *log.Options {
	o := log.DefaultOptions()

	// These scopes are, at the default "INFO" level, too chatty for command line use
	o.SetDefaultOutputLevel("dump", log.WarnLevel)
	o.SetDefaultOutputLevel("token", log.ErrorLevel)

	return o
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
		Type:          auth,
		Client:        cl,
		TrustDomain:   authTrustDomain,
		ProjectNumber: authProjectNumber,
		ClusterURL:    authClusterUrl,
	}
	args := model.Args{
		PilotAddress: pilotAddress,
		DeltaXDS:     delta,
		Metadata:     xdsMetadata,
		Client:       cl,
		Auth:         authOpts,
	}
	args, err = setDefaultArgs(args)
	if err != nil {
		return model.Args{}, err
	}
	return args, nil
}

const CLOUDRUN_ADDR = "CLOUDRUN_ADDR"

func setDefaultArgs(args model.Args) (model.Args, error) {
	if err := args.Auth.AutoPopulate(); err != nil {
		return model.Args{}, err
	}
	if _, f := xdsMetadata[CLOUDRUN_ADDR]; !f && args.Auth.Type == security.AuthTypeGoogle {
		mwh, err := args.Client.Kube().AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.Background(), "istiod-asm-managed", metav1.GetOptions{})
		if err != nil {
			return model.Args{}, fmt.Errorf("failed to default CLOUDRUN_ADDR: %v", err)
		}
		if len(mwh.Webhooks) == 0 {
			return args, nil
		}
		wh := mwh.Webhooks[0]
		if wh.ClientConfig.URL == nil {
			return model.Args{}, fmt.Errorf("failed to default CLOUDRUN_ADDR: clientConfig is not a URL")
		}
		cloudRunAddr := util.GetComponentAfter(*wh.ClientConfig.URL, "ISTIO_META_CLOUDRUN_ADDR")
		log.Infof("defaulted CLOUDRUNN_ADDR to %v", cloudRunAddr)
		xdsMetadata[CLOUDRUN_ADDR] = cloudRunAddr
	}
	return args, nil
}

var rootCmd = &cobra.Command{
	Use:          "pilot-load",
	Short:        "open XDS connections to pilot",
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return log.Configure(loggingOptions)
	},
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
		clusterCmd,
		impersonateCmd,
		startupCmd,
		xdsLatencyCmd,
		reproduceCmd,
		dumpCmd,
		isolatedCmd,
	)
}

func Execute() {
	loggingOptions.AttachCobraFlags(rootCmd)
	hiddenFlags := []string{
		"log_as_json", "log_rotate", "log_rotate_max_age", "log_rotate_max_backups",
		"log_rotate_max_size", "log_stacktrace_level", "log_target", "log_caller",
	}
	for _, opt := range hiddenFlags {
		_ = rootCmd.PersistentFlags().MarkHidden(opt)
	}
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
