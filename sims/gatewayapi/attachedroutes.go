package gatewayapi

import (
	"github.com/howardjohn/pilot-load/pkg/flag"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Config struct {
	Gateways []string
	Routes   int
}

func Command(f *pflag.FlagSet) flag.Command {
	cfg := Config{
		Routes: 100,
	}

	flag.Register(f, &cfg.Gateways, "gateways", "delay between each connection")
	return flag.Command{
		Name:        "gatewayapi-attachedroutes",
		Description: "apply routes and measure time for attachedRoutes to be valid",
		Details:     "Expected format: `kubectl get vs,gw,dr,sidecar,svc,endpoints,pod,namespace,sa -oyaml -A | kubectl grep`",
		Build: func(args *model.Args) (model.DebuggableSimulation, error) {
			return &AttachedRoutes{Config: cfg}, nil
		},
	}
}

type ApiDetails struct {
	gvk        schema.GroupVersionKind
	isIstioApi bool
}

type AttachedRoutes struct {
	Config Config
}

var _ model.Simulation = &AttachedRoutes{}

func (i *AttachedRoutes) GetConfig() any {
	return i.Config
}

func (a *AttachedRoutes) Run(ctx model.Context) error {
	// gtws := kclient.New[*gateway.Gateway](ctx.Client)
	return nil
}

func (a *AttachedRoutes) Cleanup(ctx model.Context) error {
	return nil
}
