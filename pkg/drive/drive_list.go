package drive

import (
	"context"
	"io/fs"
	"path"
	"sort"
)

func (f *FS) Stat(name string) (fs.FileInfo, error) {
	return f.StatContext(context.Background(), name)
}

func (f *FS) StatContext(ctx context.Context, name string) (Entry, error) {
	return f.statContext(ctx, name)
}

func (f *FS) resolveContext(ctx context.Context, names ...string) ([]Entry, error) {
	files := make([]Entry, 0, len(names))
	for _, name := range names {
		file, err := f.statContext(ctx, name)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}
func (f *FS) statContext(ctx context.Context, name string) (Entry, error) {
	if err := validName("stat", name); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, pathError("stat", name, err)
	}
	if name == "." {
		return f.root, nil
	}
	entry, err := f.backend.Stat(ctx, name)
	if err != nil {
		return nil, pathError("stat", name, err)
	}
	return entry, nil
}

func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	return f.ReadDirContext(context.Background(), name)
}

func (f *FS) ReadDirContext(ctx context.Context, name string) ([]fs.DirEntry, error) {
	dir, err := f.statContext(ctx, name)
	if err != nil {
		return nil, pathError("readdir", name, err)
	}
	if !dir.IsDir() {
		return nil, pathError("readdir", name, fs.ErrInvalid)
	}
	node := f.cache.load(dir.ID())
	if node == nil {
		node = f.cache.newNode(dir)
	}
	info, err := node.list(func() ([]Entry, error) {
		files, err := f.backend.List(ctx, dir, All)
		if err == nil && dir.ID() == f.root.ID() {
			for _, child := range files {
				f.cache.alias(child.ParentID(), node)
				break
			}
		}
		return files, err
	})
	if err != nil {
		return nil, pathError("readdir", name, err)
	}
	result := make([]fs.DirEntry, 0, len(info))
	for _, v := range info {
		result = append(result, v.(fs.DirEntry))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name() < result[j].Name() })
	return result, nil
}

func validName(op, name string) error {
	if fs.ValidPath(name) {
		return nil
	}
	return &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
}

func pathError(op, name string, err error) error {
	if err == nil {
		return nil
	}
	if existing, ok := err.(*fs.PathError); ok {
		err = existing.Err
	}
	return &fs.PathError{Op: op, Path: name, Err: err}
}

func parentName(name string) (string, string) {
	dir, base := path.Split(name)
	dir = path.Clean(dir)
	if dir == "/" {
		dir = "."
	} else {
		dir = path.Clean(dir)
	}
	return dir, base
}
