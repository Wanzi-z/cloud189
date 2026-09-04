package drive

import (
	"context"
	"errors"
	"fmt"
)

type familyDriveAPI interface {
	FamilyID(context.Context) (string, error)
}

type personalToFamilyCopier interface {
	CopyFromPersonal(context.Context, Entry, ...Entry) error
}

type familyToPersonalCopier interface {
	CopyToPersonal(context.Context, Entry, ...Entry) error
}

func (targetFS *FS) CopyFrom(ctx context.Context, sourceFS *FS, target string, sources ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	targetFile, err := targetFS.statContext(ctx, target)
	if err != nil {
		return err
	}
	if !targetFile.IsDir() {
		return fmt.Errorf("%s: target is not a directory", target)
	}
	sourceFiles := make([]Entry, 0, len(sources))
	for _, source := range sources {
		entry, err := sourceFS.statContext(ctx, source)
		if err != nil {
			return err
		}
		sourceFiles = append(sourceFiles, entry)
	}
	sourceFamily, sourceIsFamily := sourceFS.backend.(familyDriveAPI)
	targetFamily, targetIsFamily := targetFS.backend.(familyDriveAPI)
	if sourceIsFamily && targetIsFamily {
		sourceID, err := sourceFamily.FamilyID(ctx)
		if err != nil {
			return err
		}
		targetID, err := targetFamily.FamilyID(ctx)
		if err != nil {
			return err
		}
		if sourceID != targetID {
			return errors.New("暂不支持家庭云之间转存")
		}
		return sourceFS.backend.Copy(ctx, targetFile, sourceFiles...)
	}
	if sourceIsFamily {
		copier, ok := sourceFS.backend.(familyToPersonalCopier)
		if !ok {
			return errors.New("家庭云不支持转存到个人云")
		}
		err = copier.CopyToPersonal(ctx, targetFile, sourceFiles...)
	} else if targetIsFamily {
		copier, ok := targetFS.backend.(personalToFamilyCopier)
		if !ok {
			return errors.New("家庭云不支持从个人云转存")
		}
		err = copier.CopyFromPersonal(ctx, targetFile, sourceFiles...)
	} else {
		return errors.New("个人云复制应使用普通 cp")
	}
	if node := targetFS.cache.load(targetFile.ID()); node != nil {
		node.invalid()
	}
	return err
}
