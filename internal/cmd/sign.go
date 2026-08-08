package cmd

import (
	"github.com/gowsp/cloud189/pkg/app"
	"github.com/gowsp/cloud189/pkg/invoker"
	"github.com/spf13/cobra"
)

var signCmd = &cobra.Command{
	Use:   "sign",
	Short: "sign",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfgFile == "" {
			cfgFile = invoker.DefaultPath()
		}
		app := app.New(cfgFile)
		return app.Sign()
	},
}
