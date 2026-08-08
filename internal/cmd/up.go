package cmd

import (
	"errors"

	"github.com/gowsp/cloud189/internal/session"
	"github.com/gowsp/cloud189/pkg"
	"github.com/gowsp/cloud189/pkg/file"
	"github.com/spf13/cobra"
)

var upCfg pkg.UploadConfig
var upInput string
var upRemotePath string

func init() {
	upCmd.Flags().Uint32VarP(&upCfg.Num, "parallel", "p", 5, "并发上传数量")
	upCmd.Flags().StringVarP(&upCfg.Parten, "name", "n", "", "过滤文件名的正则表达式")
	upCmd.Flags().StringVar(&upInput, "input", "", "要上传的本地文件路径")
	upCmd.Flags().StringVar(&upRemotePath, "path", "", "上传到的云盘文件路径")
	upCmd.Flags().StringVar(&upCfg.Policy, "policy", "skip", "同名文件处理策略: skip 跳过 或 overwrite 覆盖")
}

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "上传文件",
	RunE: func(cmd *cobra.Command, args []string) error {
		if jsonOutput || upInput != "" || upRemotePath != "" {
			if len(args) != 0 {
				return cobra.NoArgs(cmd, args)
			}
			if upInput == "" {
				return errors.New("--input is required in machine mode")
			}
			if upRemotePath == "" {
				return errors.New("--path is required in machine mode")
			}
			paths := []string{upRemotePath}
			session.Parse(cmd, paths)
			if err := file.CheckPath(paths[0]); err != nil {
				return err
			}
			info, err := App().UploadFile(upCfg, upInput, paths[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return writeJSON(fileToJSONEntry(paths[0], info))
			}
			return nil
		}
		if err := cobra.MinimumNArgs(2)(cmd, args); err != nil {
			return err
		}
		length := len(args)
		cloud := session.Join(args[length-1])
		err := file.CheckPath(cloud)
		if err != nil {
			return err
		}
		locals := args[:length-1]
		if err := App().Upload(upCfg, cloud, locals...); err != nil {
			return err
		}
		return nil
	},
}
