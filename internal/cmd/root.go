package cmd

import (
	"fmt"
	"os"
	"sync"

	"github.com/gowsp/cloud189/internal/invoker"
	"github.com/gowsp/cloud189/pkg/app"
	"github.com/gowsp/cloud189/pkg/drive"
	"github.com/spf13/cobra"
)

var (
	cfgFile        string
	jsonOutput     bool
	familySelector string
	RootCmd        = &cobra.Command{
		Use:           "cloud189",
		Long:          "cloud189 是一个基于天翼云接口的命令行客户端。详情请访问 https://github.com/gowsp/cloud189",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
)

func AddCommand(cmds ...*cobra.Command) {
	RootCmd.AddCommand(cmds...)
}

func ResetTermFlags() {
	jsonOutput = false
	recursiveList = false
	dlOutput = ""
	upInput = ""
	upRemotePath = ""
	familySelector = ""
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		if jsonOutput {
			_ = writeJSONError(err)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "指定配置文件路径（默认为 $HOME/.config/cloud189/config.json）")
	RootCmd.PersistentFlags().BoolVarP(&jsonOutput, "json", "j", false, "以 JSON 格式输出结果")
	RootCmd.PersistentFlags().StringVarP(&familySelector, "family", "f", "", "指定当前命令的默认家庭云 ID")

	RootCmd.AddCommand(loginCmd)
	RootCmd.AddCommand(qrLoginCmd)
	RootCmd.AddCommand(logoutCmd)
	RootCmd.AddCommand(signCmd)
	RootCmd.AddCommand(upCmd)
	RootCmd.AddCommand(rmCmd)
	RootCmd.AddCommand(dlCmd)
	RootCmd.AddCommand(lsCmd)
	RootCmd.AddCommand(statCmd)
	RootCmd.AddCommand(mkdirCmd)
	RootCmd.AddCommand(mvCmd)
	RootCmd.AddCommand(cpCmd)
	RootCmd.AddCommand(dfCmd)
	RootCmd.AddCommand(duCmd)
	RootCmd.AddCommand(webdavCmd)
	RootCmd.AddCommand(shareCmd)
	RootCmd.AddCommand(familiesCmd)
}

var clients = make(map[string]*drive.FS)
var clientsMu sync.Mutex

func App() (*drive.FS, error) {
	return driveFor(cloudPath{familyID: familySelector, path: "/"})
}

func driveFor(location cloudPath) (*drive.FS, error) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	if cfgFile == "" {
		cfgFile = invoker.DefaultPath()
	}
	key := cfgFile + "\x00" + location.key()
	if client := clients[key]; client != nil {
		return client, nil
	}
	api, err := app.Open(cfgFile)
	if err != nil {
		return nil, err
	}
	var client *drive.FS
	if location.familyID == "" {
		client = api.Personal()
	} else {
		client, err = api.Family(RootCmd.Context(), location.familyID)
		if err != nil {
			return nil, err
		}
	}
	clients[key] = client
	return client, nil
}

func resetApp() {
	clientsMu.Lock()
	clients = make(map[string]*drive.FS)
	clientsMu.Unlock()
}
