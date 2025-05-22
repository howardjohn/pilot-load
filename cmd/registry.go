package cmd

import (
	"github.com/howardjohn/pilot-load/pkg/flag"
	"github.com/howardjohn/pilot-load/sims/podstartup"
	"github.com/howardjohn/pilot-load/sims/xdslatency"
)

var commands = []flag.CommandBuilder{
	podstartup.Command,
	xdslatency.Command,
}
