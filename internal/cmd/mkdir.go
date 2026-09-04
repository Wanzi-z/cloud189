package cmd

import (
	"github.com/gowsp/cloud189/internal/session"
	"github.com/gowsp/cloud189/pkg/file"
	"github.com/spf13/cobra"
)

var mkdirCmd = &cobra.Command{
	Use:    "mkdir",
	Short:  "创建目录",
	PreRun: session.Parse,
	Args:   cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, arg := range args {
			client, location, err := resolveCloudPath(arg)
			if err != nil {
				return err
			}
			if err := file.CheckPath(location.path); err != nil {
				return err
			}
			if err := client.Mkdir(cmd.Context(), location.name(), 0755); err != nil {
				return err
			}
		}
		return nil
	},
}
