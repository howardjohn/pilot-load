package flag

import (
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/spf13/pflag"
)

type CommandBuilder = func(f *pflag.FlagSet) Command

type Command struct {
	Name        string
	Description string
	Details     string
	Build       func(args *model.Args) (model.DebuggableSimulation, error)
}
