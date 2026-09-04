package drive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"strings"
)

func New(backend Backend) *FS {
	root := rootEntry{Entry: backend.Root()}
	return &FS{backend: backend, root: root, cache: newFileCache(root)}
}

type rootEntry struct{ Entry }

func (rootEntry) Name() string { return "." }

type FS struct {
	root    Entry
	backend Backend
	cache   *fileCache
}

func (f *FS) Space(ctx context.Context) (Space, error) {
	return f.backend.Space(ctx)
}

func (f *FS) Open(name string) (fs.File, error) {
	return f.OpenContext(context.Background(), name)
}

func (f *FS) OpenContext(ctx context.Context, name string) (fs.File, error) {
	info, err := f.statContext(ctx, name)
	if err != nil {
		return nil, pathError("open", name, err)
	}
	return f.newFile(ctx, name, info), nil
}

func (f *FS) Mkdir(ctx context.Context, name string, perm fs.FileMode) error {
	_ = perm
	if err := validName("mkdir", name); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := f.statContext(ctx, name)
	if err == nil {
		return fs.ErrExist
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	dir, err := f.backend.Mkdir(ctx, f.root, name)
	if err != nil {
		return err
	}
	if len(strings.Split(strings.Trim(name, "/"), "/")) == 1 {
		f.cache.alias(dir.ParentID(), f.cache.load(f.root.ID()))
	}
	if root := f.cache.load(f.root.ID()); root != nil {
		root.invalid()
	}
	f.cache.invalid(dir)
	return nil
}

func (f *FS) Copy(ctx context.Context, target string, source ...string) error {
	if err := validNames("copy", append([]string{target}, source...)...); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dest, err := f.statContext(ctx, target)
	if err != nil || !dest.IsDir() {
		return fmt.Errorf("%s: file does not exist or not a directory", target)
	}
	src, err := f.resolveContext(ctx, source...)
	if err != nil {
		return err
	}
	defer func() {
		if node := f.cache.load(dest.ID()); node != nil {
			node.invalid()
		}
		f.cache.invalid(src...)
	}()
	return f.backend.Copy(ctx, dest, src...)
}

func (f *FS) RemoveAll(ctx context.Context, name string) error {
	return f.remove(ctx, name)
}

func (f *FS) remove(ctx context.Context, name ...string) error {
	if err := validNames("delete", name...); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	files := make([]Entry, 0, len(name))
	for _, item := range name {
		file, err := f.statContext(ctx, item)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		return nil
	}
	err := f.backend.Remove(ctx, files...)
	for _, file := range files {
		if node := f.cache.load(file.ParentID()); node != nil {
			node.delete(file)
		}
	}
	f.cache.invalid(files...)
	return err
}

func (f *FS) Usage(ctx context.Context, name string) (Usage, error) {
	var result Usage
	fileInfo, err := f.statContext(ctx, name)
	if err != nil {
		return result, err
	}
	if !fileInfo.IsDir() {
		return Usage{Files: 1, Bytes: uint64(fileInfo.Size())}, nil
	}
	return f.backend.Usage(ctx, fileInfo)
}

func (f *FS) Put(ctx context.Context, name string, r io.Reader, size int64, options PutOptions) (Entry, error) {
	if err := validName("put", name); err != nil {
		return nil, err
	}
	parentName, base := splitName(name)
	parent, err := f.statContext(ctx, parentName)
	if err != nil {
		return nil, err
	}
	if existing, statErr := f.statContext(ctx, name); statErr == nil && !options.Overwrite {
		if existing.IsDir() {
			return nil, fmt.Errorf("%s: destination is a directory", name)
		}
		if size < 0 || existing.Size() == size {
			return existing, nil
		}
		return nil, fmt.Errorf("%s: file exists with different size", name)
	} else if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return nil, statErr
	}
	if err := f.backend.Put(ctx, parent, base, r, size, options); err != nil {
		return nil, err
	}
	f.invalidate(parent)
	return f.statContext(ctx, name)
}

func (f *FS) PutFile(ctx context.Context, name, localPath string, options PutOptions) (Entry, error) {
	local, err := os.Open(localPath)
	if err != nil {
		return nil, err
	}
	defer local.Close()
	info, err := local.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("upload source must be a regular file")
	}
	return f.Put(ctx, name, local, info.Size(), options)
}

func (f *FS) PutDigest(ctx context.Context, name string, size int64, digest Digest, options PutOptions) (Entry, error) {
	if err := validName("put", name); err != nil {
		return nil, err
	}
	parentName, base := splitName(name)
	parent, err := f.statContext(ctx, parentName)
	if err != nil {
		return nil, err
	}
	if existing, statErr := f.statContext(ctx, name); statErr == nil && !options.Overwrite {
		if existing.IsDir() {
			return nil, fmt.Errorf("%s: destination is a directory", name)
		}
		if existing.Size() == size {
			return existing, nil
		}
		return nil, fmt.Errorf("%s: file exists with different size", name)
	} else if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return nil, statErr
	}
	if err := f.backend.PutDigest(ctx, parent, base, size, digest, options); err != nil {
		return nil, err
	}
	f.invalidate(parent)
	return f.statContext(ctx, name)
}

func (f *FS) Download(ctx context.Context, name string, w io.Writer) (Entry, error) {
	entry, err := f.statContext(ctx, name)
	if err != nil {
		return nil, err
	}
	if entry.IsDir() {
		return nil, &fs.PathError{Op: "download", Path: name, Err: fs.ErrInvalid}
	}
	body, err := f.backend.Open(ctx, entry, 0)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	_, err = io.Copy(w, body)
	return entry, err
}

func (f *FS) DownloadURL(ctx context.Context, name string) (*url.URL, error) {
	entry, err := f.statContext(ctx, name)
	if err != nil {
		return nil, err
	}
	if entry.IsDir() {
		return nil, &fs.PathError{Op: "download", Path: name, Err: fs.ErrInvalid}
	}
	return f.backend.DownloadURL(ctx, entry)
}

func splitName(name string) (string, string) {
	dir, base := path.Split(name)
	if dir == "" {
		dir = "."
	} else {
		dir = strings.TrimSuffix(dir, "/")
	}
	return dir, base
}

func (f *FS) invalidate(entries ...Entry) {
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if node := f.cache.load(entry.ID()); node != nil {
			node.invalid()
		}
	}
}

func validNames(op string, names ...string) error {
	for _, name := range names {
		if err := validName(op, name); err != nil {
			return err
		}
	}
	return nil
}
