package cmd

import (
	"log"
	"net/http"

	"github.com/spf13/cobra"
)

var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "文件直链分享",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		handler, err := App().Share("/", args[1])
		if err != nil {
			return err
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/", handler)
		log.Println("启动分享服务于", args[0])
		return http.ListenAndServe(args[0], mux)
	},
}
