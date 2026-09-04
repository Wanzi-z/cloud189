package cmd

import (
	"github.com/gowsp/cloud189/internal/invoker"
	"github.com/gowsp/cloud189/pkg/app"
	"github.com/spf13/cobra"
)

var signCmd = &cobra.Command{
	Use:   "sign",
	Short: "每日签到",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfgFile == "" {
			cfgFile = invoker.DefaultPath()
		}
		client, err := app.Open(cfgFile)
		if err != nil {
			return err
		}
		return client.Sign(cmd.Context())
	},
}
