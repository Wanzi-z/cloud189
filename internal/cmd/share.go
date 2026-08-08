package cmd

import (
	"log"
	"net/http"

	"github.com/spf13/cobra"
)

var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "file direct link sharing",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		handler, err := App().Share("/", args[1])
		if err != nil {
			return err
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/", handler)
		log.Println("start share serve at", args[0])
		return http.ListenAndServe(args[0], mux)
	},
}
