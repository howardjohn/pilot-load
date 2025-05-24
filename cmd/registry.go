package cmd

import (
	"github.com/howardjohn/pilot-load/pkg/flag"
	"github.com/howardjohn/pilot-load/sims/adscimpersonate"
	"github.com/howardjohn/pilot-load/sims/cluster"
	"github.com/howardjohn/pilot-load/sims/gatewayapi"
	"github.com/howardjohn/pilot-load/sims/inmemoryistiod"
	"github.com/howardjohn/pilot-load/sims/podstartup"
	"github.com/howardjohn/pilot-load/sims/reproducecluster"
	"github.com/howardjohn/pilot-load/sims/xdslatency"
)

var commands = []flag.CommandBuilder{
	podstartup.Command,
	xdslatency.Command,
	reproducecluster.Command,
	inmemoryistiod.Command,
	adscimpersonate.Command,
	gatewayapi.Command,
	cluster.Command,
}
