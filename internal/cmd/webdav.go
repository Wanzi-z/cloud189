package cmd

import (
	"github.com/gowsp/cloud189/pkg/webdav"
	"github.com/spf13/cobra"
)

var webdavCmd = &cobra.Command{
	Use:   "webdav",
	Short: "启动 WebDAV 服务",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return webdav.Serve(args[0], App())
	},
}
