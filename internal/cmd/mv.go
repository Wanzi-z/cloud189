package cmd

import (
	"github.com/gowsp/cloud189/internal/session"
	"github.com/gowsp/cloud189/pkg/file"
	"github.com/spf13/cobra"
)

var mvCmd = &cobra.Command{
	Use:    "mv",
	Short:  "移动文件",
	PreRun: session.Parse,
	Args:   cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := file.CheckPath(args...); err != nil {
			return err
		}
		length := len(args)
		dest := args[length-1]
		from := args[:length-1]
		return App().Move(dest, from...)
	},
}
