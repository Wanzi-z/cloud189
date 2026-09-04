package cmd

import (
	"errors"
	"fmt"

	"github.com/gowsp/cloud189/internal/session"
	"github.com/gowsp/cloud189/pkg/file"
	"github.com/spf13/cobra"
)

var dlOutput string

func init() {
	dlCmd.Flags().StringVarP(&dlOutput, "output", "o", "", "下载到指定的本地路径")
}

var dlCmd = &cobra.Command{
	Use:   "dl",
	Short: "下载文件",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if jsonOutput || dlOutput != "" {
			if len(args) != 1 {
				return cobra.ExactArgs(1)(cmd, args)
			}
			session.Parse(cmd, args)
			client, location, err := resolveCloudPath(args[0])
			if err != nil {
				return err
			}
			if err := file.CheckPath(location.path); err != nil {
				return err
			}
			if dlOutput == "" {
				return errors.New("--output is required in machine mode")
			}
			info, err := downloadFile(cmd.Context(), client, location.name(), dlOutput)
			if err != nil {
				return err
			}
			if jsonOutput {
				return writeJSON(fileToJSONEntry(location.display(location.path), info))
			}
			return nil
		}
		length := len(args)
		if length < 2 {
			return cobra.MinimumNArgs(2)(cmd, args)
		}
		clouds := args[:length-1]
		session.Parse(cmd, clouds)
		local := args[length-1]
		return collectDownloadErrors(clouds, func(cloud string) error {
			client, location, err := resolveCloudPath(cloud)
			if err != nil {
				return err
			}
			if err := file.CheckPath(location.path); err != nil {
				return err
			}
			return downloadTree(cmd.Context(), client, location.name(), local)
		})
	},
}

func collectDownloadErrors(clouds []string, download func(string) error) error {
	var errs []error
	for _, cloud := range clouds {
		if err := download(cloud); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", cloud, err))
		}
	}
	return errors.Join(errs...)
}
