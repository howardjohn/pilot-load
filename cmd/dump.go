package cmd

import (
	"fmt"

	"github.com/howardjohn/pilot-load/pkg/simulation"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/spf13/cobra"
)

var dumpConfig = model.DumpConfig{}

func init() {
	dumpCmd.PersistentFlags().StringVar(&dumpConfig.Pod, "pod", dumpConfig.Pod, "pod to dump from")
	dumpCmd.PersistentFlags().StringVar(&dumpConfig.Namespace, "namespace", dumpConfig.Namespace, "namespace to dump from")
	dumpCmd.PersistentFlags().StringVar(&dumpConfig.OutputDir, "out", dumpConfig.OutputDir, "output directory")
}

var dumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "dump XDS for a pod to file, rewritten to be runnable with only files",
	RunE: func(cmd *cobra.Command, _ []string) error {
		args, err := GetArgs()
		if err != nil {
			return err
		}
		if dumpConfig.Pod == "" {
			return fmt.Errorf("--pod required")
		}
		if dumpConfig.Namespace == "" {
			return fmt.Errorf("--namespace required")
		}
		args.DumpConfig = dumpConfig
		logConfig(args.DumpConfig)
		return simulation.Dump(args)
	},
}
