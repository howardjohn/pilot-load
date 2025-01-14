package cmd

import (
	"os"
	"runtime/pprof"

	"github.com/spf13/cobra"
)

func WithProfiling(c *cobra.Command) *cobra.Command {
	var cpuProfile string
	var memProfile string
	c.PersistentFlags().StringVar(&cpuProfile, "cpuprofile", cpuProfile, "file to write cpu profile to")
	c.PersistentFlags().StringVar(&memProfile, "memprofile", memProfile, "file to write memory profile to")
	orig := c.RunE
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if cpuProfile != "" {
			f, err := os.Create(cpuProfile)
			if err != nil {
				return err
			}
			if err := pprof.StartCPUProfile(f); err != nil {
				return err
			}
			defer pprof.StopCPUProfile()
		}
		if memProfile != "" {
			f, err := os.Create(memProfile)
			if err != nil {
				return err
			}
			defer func() {
				_ = pprof.WriteHeapProfile(f)
			}()
		}
		return orig(cmd, args)
	}
	return c
}
