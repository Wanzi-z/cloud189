package cmd

import (
	"context"
	"errors"
	"net/http"

	"github.com/gowsp/cloud189/internal/session"
	"github.com/gowsp/cloud189/pkg/webdav"
	"github.com/spf13/cobra"
)

var webdavCmd = &cobra.Command{
	Use:   "webdav <address> [familyID:/]",
	Short: "启动 WebDAV 服务",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := "/"
		if len(args) == 2 {
			name = args[1]
		} else if session.Pwd() != "" {
			name = session.Pwd()
		}
		client, location, err := resolveCloudPath(name)
		if err != nil {
			return err
		}
		if len(args) == 2 && location.path != "/" {
			return errors.New("webdav 当前仅支持挂载云盘根目录")
		}
		server := &http.Server{Addr: args[0], Handler: webdav.NewHandler("", client)}
		go func() {
			<-cmd.Context().Done()
			_ = server.Shutdown(context.Background())
		}()
		return server.ListenAndServe()
	},
}
