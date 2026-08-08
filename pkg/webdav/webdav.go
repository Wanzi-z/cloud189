package webdav

import (
	"errors"
	"net/http"

	"github.com/gowsp/cloud189/pkg"
	"golang.org/x/net/webdav"
)

var errInvalidIfHeader = errors.New("webdav: invalid If header")

func Serve(addr string, client pkg.Drive) error {
	fs := &CloudFileSystem{
		app: client,
	}
	fs.handler = &webdav.Handler{
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
	}
	return http.ListenAndServe(addr, fs)
}
