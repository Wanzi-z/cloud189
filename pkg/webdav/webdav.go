package webdav

import (
	"net/http"

	"github.com/gowsp/cloud189/pkg/drive"
	"golang.org/x/net/webdav"
)

func NewFileSystem(client *drive.FS) webdav.FileSystem {
	return &fileSystem{app: client}
}

func NewHandler(prefix string, client *drive.FS) http.Handler {
	return &webdav.Handler{
		Prefix:     prefix,
		FileSystem: NewFileSystem(client),
		LockSystem: webdav.NewMemLS(),
	}
}
