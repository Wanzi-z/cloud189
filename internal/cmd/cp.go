package cmd

import (
	"errors"

	"github.com/gowsp/cloud189/internal/session"
	"github.com/gowsp/cloud189/pkg/file"
	"github.com/spf13/cobra"
)

var cpCmd = &cobra.Command{
	Use:   "cp <source...> <target-directory>",
	Short: "复制文件",
	Example: "  cloud189 cp /个人文件 123456789:/家庭目录\n" +
		"  cloud189 cp 123456789:/家庭文件 /个人目录",
	PreRun: session.Parse,
	Args:   cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		length := len(args)
		targetDrive, target, err := resolveCloudPath(args[length-1])
		if err != nil {
			return err
		}
		from := make([]string, 0, length-1)
		checked := make([]string, 0, length)
		var sourceDrive = targetDrive
		var sourceSpace cloudPath
		for index, value := range args[:length-1] {
			candidateDrive, source, err := resolveCloudPath(value)
			if err != nil {
				return err
			}
			if index == 0 {
				sourceDrive = candidateDrive
				sourceSpace = source
			} else if !source.sameSpace(sourceSpace) {
				return errors.New("一次 cp 的源文件必须来自同一个云盘")
			}
			from = append(from, source.name())
			checked = append(checked, source.path)
		}
		if err := file.CheckPath(append(checked, target.path)...); err != nil {
			return err
		}
		if sourceSpace.sameSpace(target) {
			return sourceDrive.Copy(cmd.Context(), target.name(), from...)
		}
		if sourceSpace.familyID != "" && target.familyID != "" {
			return errors.New("暂不支持家庭云之间转存")
		}
		return targetDrive.CopyFrom(cmd.Context(), sourceDrive, target.name(), from...)
	},
}
