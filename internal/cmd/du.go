package cmd

import (
	"fmt"
	pathpkg "path"

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
		var path string
		if len(args) == 0 {
			path = session.Pwd()
		} else {
			path = args[0]
		}

		client, location, err := resolveCloudPath(path)
		if err != nil {
			return err
		}
		path = location.name()
		if err := file.CheckPath(location.path); err != nil {
			return err
		}
		fileInfo, err := client.StatContext(cmd.Context(), path)
		if err != nil {
			return err
		}

		if fileInfo.IsDir() {
			files, err := client.ReadDirContext(cmd.Context(), path)
			if err != nil {
				return err
			}

			fmt.Printf("%-10s %-10s %-10s %s\n", "文件数", "目录数", "大小", "名称")

			totalFiles := uint64(0)
			totalSize := uint64(0)
			totalFolders := uint64(0)

			for _, v := range files {
				info, _ := v.Info()
				usage, err := client.Usage(cmd.Context(), pathpkg.Join(path, info.Name()))
				if err != nil {
					return err
				}

				fmt.Printf("%-10d %-10d %-10s %s\n",
					usage.Files,
					usage.Directories,
					file.ReadableSize(usage.Bytes),
					info.Name())

				totalFiles += usage.Files
				totalSize += usage.Bytes
				totalFolders += usage.Directories
			}

			fmt.Printf("%-10d %-10d %-10s %s\n", totalFiles, totalFolders, file.ReadableSize(totalSize), "合计")
		} else {
			usage, err := client.Usage(cmd.Context(), path)
			if err != nil {
				return err
			}

			fmt.Printf("%-10s %-10s %-10s %s\n", "文件数", "目录数", "大小", "名称")
			fmt.Printf("%-10d %-10d %-10s %s\n",
				usage.Files,
				usage.Directories,
				file.ReadableSize(usage.Bytes),
				fileInfo.Name())
		}
		return nil
	},
}
