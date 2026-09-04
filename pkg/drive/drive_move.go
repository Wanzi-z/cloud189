package drive

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
)

func (f *FS) Rename(ctx context.Context, oldName, newName string) error {
	return f.move(ctx, newName, oldName)
}

func (f *FS) move(ctx context.Context, target string, source ...string) error {
	if err := validNames("move", append([]string{target}, source...)...); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(source) == 1 {
		return f.singleMove(ctx, target, source[0])
	}
	return f.multiMove(ctx, target, source...)
}

func (f *FS) singleMove(ctx context.Context, target string, sources string) error {
	if path.Clean(target) == path.Clean(sources) {
		return nil
	}
	files, err := f.resolveContext(ctx, sources)
	if err != nil {
		return err
	}
	source := files[0]
	dest, err := f.statContext(ctx, target)
	defer func() {
		f.cache.invalid(source, dest)
	}()
	if err == nil {
		return pathError("rename", target, fs.ErrExist)
	}
	if errors.Is(err, fs.ErrNotExist) {
		dir, name := parentName(target)
		parent, err := f.statContext(ctx, dir)
		if err != nil {
			return err
		}
		if source.Name() != name && source.ParentID() != parent.ID() {
			collision, collisionErr := f.statContext(ctx, path.Join(dir, source.Name()))
			if collisionErr == nil && collision.ID() != source.ID() {
				oldName := source.Name()
				if err := f.backend.Rename(ctx, source, name); err != nil {
					return err
				}
				if err := f.backend.Move(ctx, parent, source); err != nil {
					_ = f.backend.Rename(ctx, source, oldName)
					return err
				}
				return nil
			}
			if collisionErr != nil && !errors.Is(collisionErr, fs.ErrNotExist) {
				return collisionErr
			}
		}
		if err := f.backend.Move(ctx, parent, source); err != nil {
			return err
		}
		if source.Name() == name {
			return nil
		}
		return f.backend.Rename(ctx, source, name)
	}
	return err
}
func (f *FS) multiMove(ctx context.Context, target string, source ...string) error {
	dest, err := f.statContext(ctx, target)
	if err != nil {
		return err
	}
	if !dest.IsDir() {
		return fmt.Errorf("target '%s' is not a directory", target)
	}
	files, err := f.resolveContext(ctx, source...)
	if err != nil {
		return err
	}
	defer func() {
		if node := f.cache.load(dest.ID()); node != nil {
			node.invalid()
		}
		f.cache.invalid(files...)
	}()
	return f.backend.Move(ctx, dest, files...)
}
