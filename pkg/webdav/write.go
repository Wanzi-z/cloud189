package webdav

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sync"

	"github.com/gowsp/cloud189/pkg/drive"
)

type write struct {
	ctx    context.Context
	app    *drive.FS
	name   string
	temp   *os.File
	commit bool
	once   sync.Once
	err    error
}

func newWrite(ctx context.Context, app *drive.FS, name string, flag int, perm os.FileMode) (*write, error) {
	existing, err := app.StatContext(ctx, name)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if exists && existing.IsDir() {
		return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrInvalid}
	}
	if exists && flag&os.O_EXCL != 0 && flag&os.O_CREATE != 0 {
		return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrExist}
	}
	if !exists {
		if flag&os.O_CREATE == 0 {
			return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrNotExist}
		}
		parent, err := app.StatContext(ctx, path.Dir(name))
		if err != nil {
			return nil, err
		}
		if !parent.IsDir() {
			return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrInvalid}
		}
	}
	temp, err := os.CreateTemp("", "cloud189-webdav-write-*")
	if err != nil {
		return nil, err
	}
	w := &write{ctx: ctx, app: app, name: name, temp: temp, commit: !exists || flag&os.O_TRUNC != 0}
	if exists && flag&os.O_TRUNC == 0 {
		if _, err := app.Download(ctx, name, temp); err != nil {
			_ = w.cleanup()
			return nil, err
		}
		if _, err := temp.Seek(0, io.SeekStart); err != nil {
			_ = w.cleanup()
			return nil, err
		}
	}
	_ = perm
	return w, nil
}

func (w *write) Read(p []byte) (int, error) { return w.temp.Read(p) }
func (w *write) Seek(offset int64, whence int) (int64, error) {
	return w.temp.Seek(offset, whence)
}
func (w *write) Write(p []byte) (int, error) {
	w.commit = true
	return w.temp.Write(p)
}
func (w *write) Readdir(int) ([]fs.FileInfo, error) {
	return nil, &os.PathError{Op: "readdir", Path: w.name, Err: os.ErrInvalid}
}
func (w *write) Stat() (fs.FileInfo, error) {
	info, err := w.temp.Stat()
	if err != nil {
		return nil, err
	}
	return namedInfo{FileInfo: info, name: path.Base(w.name)}, nil
}
func (w *write) Close() error {
	w.once.Do(func() {
		if w.commit {
			info, err := w.temp.Stat()
			if err == nil {
				_, err = w.temp.Seek(0, io.SeekStart)
			}
			if err == nil {
				_, err = w.app.Put(w.ctx, w.name, w.temp, info.Size(), drive.PutOptions{Overwrite: true})
			}
			w.err = err
		}
		w.err = errors.Join(w.err, w.cleanup())
	})
	return w.err
}
func (w *write) cleanup() error {
	name := w.temp.Name()
	return errors.Join(w.temp.Close(), os.Remove(name))
}

type namedInfo struct {
	fs.FileInfo
	name string
}

func (i namedInfo) Name() string { return i.name }
