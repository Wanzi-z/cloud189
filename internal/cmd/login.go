package cmd

import (
	"fmt"

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
			return loginFunc(args[0], args[1])
		}
		line := liner.NewLiner()
		defer line.Close()
		username, _ := line.Prompt("用户名: ")
		password, _ := line.PasswordPrompt("密码: ")
		return loginFunc(username, password)
	},
}

var qrLoginCmd = &cobra.Command{
	Use:   "qrlogin",
	Short: "扫码登录天翼云盘",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := App().QrLogin(); err != nil {
			return err
		}
		fmt.Println("登录成功")
		return nil
	},
}

func loginFunc(username, password string) error {
	if err := App().Login(username, password); err != nil {
		return err
	}
	fmt.Println("登录成功")
	return nil
}

func init() {
	loginCmd.Flags().BoolVarP(&usePwd, "i", "i", false, "通过输入用户名和密码登录")
}
