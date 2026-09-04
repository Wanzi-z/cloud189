package webdav

import (
	"context"
	"os"
	"path"
	"strings"

	"golang.org/x/net/webdav"

	"github.com/gowsp/cloud189/pkg/drive"
)

type fileSystem struct {
	app *drive.FS
}

func (f *fileSystem) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	native, err := nativeName(name)
	if err != nil {
		return err
	}
	return f.app.Mkdir(ctx, native, perm)
}
func (f *fileSystem) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	native, err := nativeName(name)
	if err != nil {
		return nil, err
	}
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC) != 0 {
		return newWrite(ctx, f.app, native, flag, perm)
	}
	return newRead(ctx, f.app, native)
}
func (f *fileSystem) RemoveAll(ctx context.Context, name string) error {
	native, err := nativeName(name)
	if err != nil {
		return err
	}
	return f.app.RemoveAll(ctx, native)
}
func (f *fileSystem) Rename(ctx context.Context, oldName, newName string) error {
	oldNative, err := nativeName(oldName)
	if err != nil {
		return err
	}
	newNative, err := nativeName(newName)
	if err != nil {
		return err
	}
	return f.app.Rename(ctx, oldNative, newNative)
}
func (f *fileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	native, err := nativeName(name)
	if err != nil {
		return nil, err
	}
	return f.app.StatContext(ctx, native)
}

func nativeName(name string) (string, error) {
	if !strings.HasPrefix(name, "/") {
		return "", &os.PathError{Op: "webdav", Path: name, Err: os.ErrInvalid}
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", &os.PathError{Op: "webdav", Path: name, Err: os.ErrInvalid}
		}
	}
	name = path.Clean(name)
	if name == "/" {
		return ".", nil
	}
	return strings.TrimPrefix(name, "/"), nil
}
