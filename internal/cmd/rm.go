package cmd

import (
	"github.com/gowsp/cloud189/internal/session"
	"github.com/gowsp/cloud189/pkg/file"
	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:    "rm",
	Short:  "remove file",
	PreRun: session.Parse,
	Args:   cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := file.CheckPath(args...); err != nil {
			return err
		}
		return App().Delete(args...)
	},
}
