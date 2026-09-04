package cmd

import (
	"fmt"

	"github.com/gowsp/cloud189/internal/invoker"
	"github.com/gowsp/cloud189/pkg/app"
	"github.com/spf13/cobra"
)

var familiesCmd = &cobra.Command{
	Use:     "fam",
	Aliases: []string{"families"},
	Short:   "列出家庭云",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := cfgFile
		if path == "" {
			path = invoker.DefaultPath()
		}
		client, err := app.Open(path)
		if err != nil {
			return err
		}
		families, err := client.Families(cmd.Context())
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeJSON(families)
		}
		fmt.Printf("%-20s %-24s %-8s %s\n", "FAMILY_ID", "REMARK", "MEMBERS", "ROLE")
		for _, family := range families {
			fmt.Printf("%-20s %-24s %-8d %d\n", family.FamilyID(), family.RemarkName, family.Count, family.UserRole)
		}
		return nil
	},
}
