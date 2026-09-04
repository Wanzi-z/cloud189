package cmd

import (
	"errors"

	"github.com/gowsp/cloud189/internal/session"
	"github.com/gowsp/cloud189/pkg/file"
	"github.com/spf13/cobra"
)

var upCfg uploadConfig
var upInput string
var upRemotePath string

func init() {
	upCmd.Flags().Uint32VarP(&upCfg.Parallel, "parallel", "p", 5, "并发上传数量")
	upCmd.Flags().StringVarP(&upCfg.Pattern, "name", "n", "", "过滤文件名的正则表达式")
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
			client, location, err := resolveCloudPath(paths[0])
			if err != nil {
				return err
			}
			if err := file.CheckPath(location.path); err != nil {
				return err
			}
			options, err := upCfg.options()
			if err != nil {
				return err
			}
			info, err := putFile(cmd.Context(), client, upInput, location.name(), options)
			if err != nil {
				return err
			}
			if jsonOutput {
				return writeJSON(fileToJSONEntry(location.display(location.path), info))
			}
			return nil
		}
		if err := cobra.MinimumNArgs(2)(cmd, args); err != nil {
			return err
		}
		length := len(args)
		cloud := session.Join(args[length-1])
		client, location, err := resolveCloudPath(cloud)
		if err != nil {
			return err
		}
		err = file.CheckPath(location.path)
		if err != nil {
			return err
		}
		locals := args[:length-1]
		if err := uploadInputs(cmd.Context(), client, location.name(), locals, upCfg); err != nil {
			return err
		}
		return nil
	},
}
