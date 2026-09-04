package cmd

import (
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/gowsp/cloud189/internal/session"
	"github.com/spf13/cobra"
)

var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "文件直链分享",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, location, err := resolveCloudPath(session.Join(args[1]))
		if err != nil {
			return err
		}
		base := location.name()
		if _, err := client.StatContext(cmd.Context(), base); err != nil {
			return err
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			name := path.Join(base, strings.TrimPrefix(r.URL.Path, "/"))
			entry, err := client.StatContext(r.Context(), name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if entry.IsDir() {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			target, err := client.DownloadURL(r.Context(), name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			http.Redirect(w, r, target.String(), http.StatusFound)
		})
		log.Println("启动分享服务于", args[0])
		return http.ListenAndServe(args[0], mux)
	},
}
