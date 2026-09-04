package cmd

import (
	"fmt"

	"github.com/gowsp/cloud189/internal/session"
	"github.com/gowsp/cloud189/pkg/file"
	"github.com/spf13/cobra"
)

var dfCmd = &cobra.Command{
	Use:   "df [path|familyID:]",
	Short: "显示空间使用情况",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := "/"
		if len(args) > 0 {
			name = session.Join(args[0])
		} else if session.Pwd() != "" {
			name = session.Pwd()
		}
		client, _, err := resolveCloudPath(name)
		if err != nil {
			return err
		}
		space, err := client.Space(cmd.Context())
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeJSON(JSONSpace{
				OK:        true,
				Capacity:  space.Capacity,
				Available: space.Available,
			})
		}
		capacity := space.Capacity
		available := space.Available
		used := capacity - available
		fmt.Printf("%-12s%-12s%-12s%s\n", "Size", "Used", "Avail", "Use%")
		fmt.Printf("%-12s%-12s%-12s%.2f%%\n",
			file.ReadableSize(capacity),
			file.ReadableSize(used),
			file.ReadableSize(available),
			float64(used)*100/float64(capacity),
		)
		return nil

	},
}
