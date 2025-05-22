package podstartup

import (
	"fmt"
	"os"
	"time"

	"github.com/howardjohn/pilot-load/pkg/flag"
	"github.com/spf13/pflag"

	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type StartupConfig struct {
	Namespace   string
	Concurrency int
	Inject      bool
	Cooldown    time.Duration
	Spec        string
}

func Command(f *pflag.FlagSet) flag.Command {
	startupConfig := model.StartupConfig{
		Concurrency: 1,
		Cooldown:    time.Millisecond * 10,
	}

	var specFile string
	flag.Register(f, &startupConfig.Inject, "inject", "if true, we will inject the pod")
	flag.Register(f, &startupConfig.Concurrency, "concurrency", "number of pods to start concurrently")
	flag.Register(f, &startupConfig.Namespace, "namespace", "namespace to run in")
	flag.Register(f, &startupConfig.Cooldown, "cooldown", "time to wait after starting each pod (per worker)")
	flag.Register(f, &specFile, "spec", "pod spec")
	return flag.Command{
		Name:        "pod-startup",
		Description: "measure the time for pods to start",
		Build: func() (model.DebuggableSimulation, error) {
			if startupConfig.Namespace == "" {
				return nil, fmt.Errorf("--namespace required")
			}
			if specFile == "" {
				return nil, fmt.Errorf("--spec required")
			}
			f, err := os.ReadFile(specFile)
			if err != nil {
				return nil, err
			}
			startupConfig.Spec = string(f)

			return &simulation.PodStartupSimulation{Config: startupConfig}, nil
		},
	}
}
