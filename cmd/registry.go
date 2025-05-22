package cmd

import (
	"github.com/howardjohn/pilot-load/pkg/flag"
	"github.com/howardjohn/pilot-load/sims/podstartup"
)

var commands = []flag.CommandBuilder {
	podstartup.Command,
}
