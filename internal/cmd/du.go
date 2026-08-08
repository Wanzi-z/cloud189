package cmd

import (
	"fmt"

	"github.com/gowsp/cloud189/internal/session"
	"github.com/gowsp/cloud189/pkg/file"
	"github.com/spf13/cobra"
)

var duCmd = &cobra.Command{
	Use:    "du",
	Short:  "显示文件占用统计",
	PreRun: session.Parse,
	Args:   cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := file.CheckPath(args...); err != nil {
			return err
		}

		var path string
		if len(args) == 0 {
			path = session.Pwd()
		} else {
			path = args[0]
		}

		fileInfo, err := App().Stat(path)
		if err != nil {
			return err
		}

		if fileInfo.IsDir() {
			files, err := App().ReadDir(path)
			if err != nil {
				return err
			}

			fmt.Printf("%-10s %-10s %-10s %s\n", "文件数", "目录数", "大小", "名称")

			totalFiles := uint64(0)
			totalSize := uint64(0)
			totalFolders := uint64(0)

			for _, v := range files {
				info, _ := v.Info()
				usage, err := App().Usage(path + "/" + info.Name())
				if err != nil {
					return err
				}

				fmt.Printf("%-10d %-10d %-10s %s\n",
					usage.FileCount(),
					usage.FolderCount(),
					file.ReadableSize(usage.FileSize()),
					info.Name())

				totalFiles += usage.FileCount()
				totalSize += usage.FileSize()
				totalFolders += usage.FolderCount()
			}

			fmt.Printf("%-10d %-10d %-10s %s\n", totalFiles, totalFolders, file.ReadableSize(totalSize), "合计")
		} else {
			usage, err := App().Usage(path)
			if err != nil {
				return err
			}

			fmt.Printf("%-10s %-10s %-10s %s\n", "文件数", "目录数", "大小", "名称")
			fmt.Printf("%-10d %-10d %-10s %s\n",
				usage.FileCount(),
				usage.FolderCount(),
				file.ReadableSize(usage.FileSize()),
				fileInfo.Name())
		}
		return nil
	},
}