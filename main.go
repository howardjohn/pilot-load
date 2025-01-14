package main

import (
	"istio.io/istio/pkg/log"

	"github.com/howardjohn/pilot-load/cmd"
)

func main() {
	log.EnableKlogWithCobra()
	cmd.Execute()
}
