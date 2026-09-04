package cmd

import (
	"context"
	"fmt"

	"github.com/gowsp/cloud189/internal/invoker"
	"github.com/gowsp/cloud189/pkg/app"
	"github.com/peterh/liner"
	"github.com/spf13/cobra"
)

var usePwd bool

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "登录天翼云盘",
	Args: func(cmd *cobra.Command, args []string) error {
		if usePwd && len(args) < 2 {
			return fmt.Errorf("requires username password parameter, received %d", len(args))
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if usePwd {
			return loginFunc(cmd.Context(), args[0], args[1])
		}
		line := liner.NewLiner()
		defer line.Close()
		username, _ := line.Prompt("用户名: ")
		password, _ := line.PasswordPrompt("密码: ")
		return loginFunc(cmd.Context(), username, password)
	},
}

var qrLoginCmd = &cobra.Command{
	Use:   "qrlogin",
	Short: "扫码登录天翼云盘",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfgFile == "" {
			cfgFile = invoker.DefaultPath()
		}
		client, err := app.Open(cfgFile)
		if err != nil {
			return err
		}
		if err := client.QRLogin(cmd.Context()); err != nil {
			return err
		}
		resetApp()
		fmt.Println("登录成功")
		return nil
	},
}

func loginFunc(ctx context.Context, username, password string) error {
	if cfgFile == "" {
		cfgFile = invoker.DefaultPath()
	}
	client, err := app.Open(cfgFile)
	if err != nil {
		return err
	}
	if err := client.Login(ctx, username, password); err != nil {
		return err
	}
	resetApp()
	fmt.Println("登录成功")
	return nil
}

func init() {
	loginCmd.Flags().BoolVarP(&usePwd, "i", "i", false, "通过输入用户名和密码登录")
}
