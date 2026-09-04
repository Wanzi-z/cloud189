package webdav

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"sync"

	"github.com/gowsp/cloud189/pkg/drive"
	"golang.org/x/net/webdav"
)

func newRead(ctx context.Context, app *drive.FS, name string) (webdav.File, error) {
	stat, err := app.StatContext(ctx, name)
	if err != nil {
		return nil, err
	}
	return &read{ctx: ctx, app: app, name: name, stat: stat.(drive.Entry)}, nil
}

type read struct {
	ctx     context.Context
	app     *drive.FS
	name    string
	stat    drive.Entry
	load    sync.Once
	temp    *os.File
	err     error
	entries []fs.FileInfo
	offset  int
}

func (r *read) getTemp() (*os.File, error) {
	r.load.Do(func() {
		temp, err := os.CreateTemp("", "cloud189-webdav-read-*")
		if err != nil {
			r.err = err
			return
		}
		name := temp.Name()
		if err := temp.Close(); err != nil {
			r.err = err
			_ = os.Remove(name)
			return
		}
		destination, err := os.OpenFile(name, os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			r.err = err
			_ = os.Remove(name)
			return
		}
		_, downloadErr := r.app.Download(r.ctx, r.name, destination)
		if err := errors.Join(downloadErr, destination.Close()); err != nil {
			r.err = err
			_ = os.Remove(name)
			return
		}
		r.temp, r.err = os.OpenFile(name, os.O_RDWR, 0600)
	})
	return r.temp, r.err
}
func (r *read) Seek(offset int64, whence int) (int64, error) {
	temp, err := r.getTemp()
	if err != nil {
		return 0, err
	}
	return temp.Seek(offset, whence)
}
func (r *read) Read(p []byte) (n int, err error) {
	temp, err := r.getTemp()
	if err != nil {
		return 0, err
	}
	return temp.Read(p)
}
func (r *read) Write(p []byte) (n int, err error) {
	temp, err := r.getTemp()
	if err != nil {
		return 0, err
	}
	return temp.Write(p)
}
func (r *read) Close() error {
	if r.temp == nil {
		return r.err
	}
	return errors.Join(r.temp.Close(), os.Remove(r.temp.Name()))
}
func (r *read) Readdir(count int) ([]fs.FileInfo, error) {
	if !r.stat.IsDir() {
		return nil, &os.PathError{Op: "readdir", Path: r.name, Err: os.ErrInvalid}
	}
	if r.entries == nil {
		data, err := r.app.ReadDirContext(r.ctx, r.name)
		if err != nil {
			return nil, err
		}
		r.entries = make([]fs.FileInfo, len(data))
		for i, entry := range data {
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			r.entries[i] = info
		}
	}
	if r.offset >= len(r.entries) && count > 0 {
		return nil, io.EOF
	}
	end := len(r.entries)
	if count > 0 && r.offset+count < end {
		end = r.offset + count
	}
	files := r.entries[r.offset:end]
	r.offset = end
	return files, nil
}
func (r *read) Stat() (info fs.FileInfo, err error) {
	return r.stat, err
}
