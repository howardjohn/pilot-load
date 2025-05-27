package flag

import (
	"fmt"
	"os"

	"github.com/howardjohn/pilot-load/pkg/simulation/security"
	"github.com/spf13/cobra"
	"istio.io/istio/pkg/log"
)

var (
	pilotAddress   = defaultAddress()
	xdsMetadata    = map[string]string{}
	auth           = string(security.AuthTypeDefault)
	delta          = true
	kubeconfig     = os.Getenv("KUBECONFIG")
	loggingOptions = defaultLogOptions()

	qps = 100000
)

func defaultLogOptions() *log.Options {
	o := log.DefaultOptions()

	// These scopes are, at the default "INFO" level, too chatty for command line use
	o.SetDefaultOutputLevel("dump", log.WarnLevel)
	o.SetDefaultOutputLevel("token", log.ErrorLevel)

	return o
}


func defaultAddress() string {
	_, inCluster := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	if inCluster {
		return "istiod.istio-system.svc:15010"
	}
	return "localhost:15010"
}

func AttachGlobalFlags(c *cobra.Command) {
	c.PersistentFlags().StringVarP(&pilotAddress, "pilot-address", "p", pilotAddress, "address to pilot")
	c.PersistentFlags().StringVarP(&auth, "auth", "a", auth,
		fmt.Sprintf("auth type use. If not set, default based on port number. Supported options: %v", security.AuthTypeOptions()))
	c.PersistentFlags().StringVarP(&kubeconfig, "kubeconfig", "k", kubeconfig, "kubeconfig")
	c.PersistentFlags().IntVar(&qps, "qps", qps, "qps for kube client")
	c.PersistentFlags().StringToStringVarP(&xdsMetadata, "metadata", "m", xdsMetadata, "xds metadata")

	c.PersistentFlags().BoolVar(&delta, "delta", delta, "use delta XDS")

	loggingOptions.AttachCobraFlags(c)
	hiddenFlags := []string{
		"log_as_json", "log_rotate", "log_rotate_max_age", "log_rotate_max_backups",
		"log_rotate_max_size", "log_stacktrace_level", "log_target", "log_caller",
	}
	for _, opt := range hiddenFlags {
		_ = c.PersistentFlags().MarkHidden(opt)
	}
}