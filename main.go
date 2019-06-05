package main

import (
	"os"

	"github.com/howardjohn/file-based-istio/cmd"
)

var (
	OUTDIR = os.Getenv("OUTDIR")
)

func main() {
	cmd.Execute()
}
