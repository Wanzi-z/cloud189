package cmd

import (
	"errors"
	"github.com/gowsp/cloud189/internal/session"
	"github.com/gowsp/cloud189/pkg/file"
	"github.com/spf13/cobra"
	"path"
)

var mvCmd = &cobra.Command{
	Use:    "mv",
	Short:  "移动文件",
	PreRun: session.Parse,
	Args:   cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		length := len(args)
		drive, target, err := resolveCloudPath(args[length-1])
		if err != nil {
			return err
		}
		from := make([]string, 0, length-1)
		checked := make([]string, 0, length)
		for _, value := range args[:length-1] {
			_, source, err := resolveCloudPath(value)
			if err != nil {
				return err
			}
			if !source.sameSpace(target) {
				return errors.New("不支持跨云盘移动，请使用 cp 转存后再删除源文件")
			}
			from = append(from, source.name())
			checked = append(checked, source.path)
		}
		if err := file.CheckPath(append(checked, target.path)...); err != nil {
			return err
		}
		if len(from) == 1 {
			destination := target.name()
			if info, statErr := drive.StatContext(cmd.Context(), destination); statErr == nil && info.IsDir() {
				destination = path.Join(destination, path.Base(from[0]))
			}
			return drive.Rename(cmd.Context(), from[0], destination)
		}
		for _, source := range from {
			if err := drive.Rename(cmd.Context(), source, path.Join(target.name(), path.Base(source))); err != nil {
				return err
			}
		}
		return nil
	},
}
