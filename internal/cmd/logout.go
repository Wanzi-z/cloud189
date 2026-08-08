package cmd

import (
	"fmt"
	"os"

	"github.com/gowsp/cloud189/pkg/invoker"
	"github.com/peterh/liner"
	"github.com/spf13/cobra"
)

var confirm bool

var logoutCmd = &cobra.Command{
	Use:          "logout",
	Short:        "退出登录",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if confirm {
			return logout()
		}
		liner := liner.NewLiner()
		defer liner.Close()
		reply, err := liner.Prompt("确定要退出登录吗？(y/n) ")
		if err != nil {
			return err
		}
		switch reply {
		case "y", "Y":
			return logout()
		}
		return nil
	},
}

func logout() error {
	if cfgFile == "" {
		cfgFile = invoker.DefaultPath()
	}
	err := os.Remove(cfgFile)
	if os.IsNotExist(err) {
		err = nil
	}
	if err == nil {
		fmt.Println("退出登录成功")
	}
	return err
}

func init() {
	logoutCmd.Flags().BoolVarP(&confirm, "f", "f", false, "直接退出，不确认")
}
