package cmd

import (
	"github.com/gowsp/cloud189/internal/session"
	"github.com/gowsp/cloud189/pkg/drive"
	"github.com/gowsp/cloud189/pkg/file"
	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:    "rm",
	Short:  "删除文件",
	PreRun: session.Parse,
	Args:   cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		groups := make(map[*drive.FS][]string)
		for _, arg := range args {
			client, location, err := resolveCloudPath(arg)
			if err != nil {
				return err
			}
			if err := file.CheckPath(location.path); err != nil {
				return err
			}
			groups[client] = append(groups[client], location.name())
		}
		for client, names := range groups {
			if err := client.Remove(cmd.Context(), names...); err != nil {
				return err
			}
		}
		return nil
	},
}
