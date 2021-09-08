package main

import (
	"github.com/howardjohn/pilot-load/cmd"

	"istio.io/pkg/log"
)

func main() {
	log.EnableKlogWithCobra()
	cmd.Execute()
}
