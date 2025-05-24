package inmemoryistiod

import (
	"fmt"

	"github.com/howardjohn/pilot-load/pkg/flag"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/spf13/pflag"
)

func Command(f *pflag.FlagSet) flag.Command {
	return flag.Command{
		Name:        "in-memory-istiod",
		Description: "Run an in-memory Istiod implementation and pass it the provided config",
		Build: func(args *model.Args) (model.DebuggableSimulation, error) {
			return nil, fmt.Errorf("this is not implemented anymore; see the 'isolated' command on older branches")
		},
	}
}
