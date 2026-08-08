package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var date string
var version string

var versionCmd = &cobra.Command{
	Use:   "version",
		Short: "打印版本信息",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("构建时间: %s\n", date)
			fmt.Printf("版本: %s\n", version)
		},
}
