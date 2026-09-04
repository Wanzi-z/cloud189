package drive

import (
	"context"
	"io"
	"io/fs"
)

func (f *FS) newFile(ctx context.Context, name string, info Entry) fs.File {
	if info.IsDir() {
		return &dirHandle{fs: f, ctx: ctx, name: name, info: info}
	}
	return &fileHandle{fs: f, ctx: ctx, info: info}
}

type fileHandle struct {
	fs   *FS
	ctx  context.Context
	info Entry
	body io.ReadCloser
}

func (a *fileHandle) Stat() (fs.FileInfo, error) { return a.info, nil }
func (a *fileHandle) Read(p []byte) (int, error) {
	if a.body == nil {
		if err := a.ctx.Err(); err != nil {
			return 0, err
		}
		body, err := a.fs.backend.Open(a.ctx, a.info, 0)
		if err != nil {
			return 0, err
		}
		a.body = body
	}
	return a.body.Read(p)
}
func (a *fileHandle) Close() error {
	if a.body != nil {
		return a.body.Close()
	}
	return nil
}

type dirHandle struct {
	fs      *FS
	ctx     context.Context
	name    string
	info    Entry
	entries []fs.DirEntry
	offset  int
}

func (a *dirHandle) Stat() (fs.FileInfo, error) { return a.info, nil }
func (a *dirHandle) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: a.name, Err: fs.ErrInvalid}
}
func (a *dirHandle) Close() error { return nil }
func (a *dirHandle) ReadDir(n int) ([]fs.DirEntry, error) {
	if a.entries == nil {
		entries, err := a.fs.ReadDirContext(a.ctx, a.name)
		if err != nil {
			return nil, err
		}
		a.entries = entries
	}
	if a.offset >= len(a.entries) && n > 0 {
		return nil, io.EOF
	}
	end := len(a.entries)
	if n > 0 && a.offset+n < end {
		end = a.offset + n
	}
	result := a.entries[a.offset:end]
	a.offset = end
	return result, nil
}
