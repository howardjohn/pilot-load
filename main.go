package main

import (
	"github.com/howardjohn/pilot-load/cmd"

	"istio.io/istio/pkg/log"
)

func main() {
	log.EnableKlogWithCobra()
	cmd.Execute()
}
