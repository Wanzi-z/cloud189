package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"

	"github.com/gowsp/cloud189/internal/invoker"
	pkg "github.com/gowsp/cloud189/pkg/drive"
)

func (f *familyAPI) Copy(ctx context.Context, target pkg.Entry, sources ...pkg.Entry) error {
	return f.CopyPolicy(ctx, pkg.ConflictError, target, sources...)
}

func (f *familyAPI) CopyPolicy(ctx context.Context, policy pkg.ConflictPolicy, target pkg.Entry, sources ...pkg.Entry) error {
	filtered := make([]pkg.Entry, 0, len(sources))
	for _, source := range sources {
		if source == nil {
			continue
		}
		if f.isRoot(target) {
			if rootID := f.rootID(); rootID != "" && source.ParentID() == rootID {
				continue
			}
		} else if source.ParentID() == target.ID() {
			continue
		}
		filtered = append(filtered, source)
	}
	id, err := f.familyTargetID(target)
	if err != nil {
		return err
	}
	batch := f.familyBatch()
	batch.conflict = toConflictPolicy(policy)
	return batch.run(ctx, "COPY", id, filtered...)
}

func (f *familyAPI) Move(ctx context.Context, target pkg.Entry, sources ...pkg.Entry) error {
	return f.MovePolicy(ctx, pkg.ConflictError, target, sources...)
}

func (f *familyAPI) MovePolicy(ctx context.Context, policy pkg.ConflictPolicy, target pkg.Entry, sources ...pkg.Entry) error {
	filtered := make([]pkg.Entry, 0, len(sources))
	for _, source := range sources {
		if source == nil {
			continue
		}
		if f.isRoot(target) {
			if rootID := f.rootID(); rootID != "" && source.ParentID() == rootID {
				continue
			}
		} else if source.ParentID() == target.ID() {
			continue
		}
		filtered = append(filtered, source)
	}
	id, err := f.familyTargetID(target)
	if err != nil {
		return err
	}
	batch := f.familyBatch()
	batch.conflict = toConflictPolicy(policy)
	return batch.run(ctx, "MOVE", id, filtered...)
}

func (f *familyAPI) Delete(ctx context.Context, sources ...pkg.Entry) error {
	return f.familyBatch().run(ctx, "DELETE", nil, sources...)
}

func (f *familyAPI) familyTargetID(target pkg.Entry) (*string, error) {
	if target == nil || !target.IsDir() {
		return nil, errors.New("target is not a directory")
	}
	id := target.ID()
	if f.isRoot(target) {
		id = "0"
	}
	return &id, nil
}

// CopyFromPersonal transfers personal entries into family storage (copyType=1).
func (f *familyAPI) CopyFromPersonal(ctx context.Context, target pkg.Entry, sources ...pkg.Entry) error {
	return f.createTransferTask(ctx, "1", invoker.PersonalScope, false, target, sources...)
}

// CopyToPersonal transfers family entries into personal storage (copyType=2).
func (f *familyAPI) CopyToPersonal(ctx context.Context, target pkg.Entry, sources ...pkg.Entry) error {
	return f.createTransferTask(ctx, "2", invoker.FamilyScope, true, target, sources...)
}

func (f *familyAPI) createTransferTask(ctx context.Context, copyType string, scope invoker.Scope, pollWithFamily bool, target pkg.Entry, sources ...pkg.Entry) error {
	if len(sources) == 0 {
		return nil
	}
	if err := f.ensure(ctx); err != nil {
		return err
	}
	infos := make([]batchTaskInfo, 0, len(sources))
	for _, source := range sources {
		if source == nil {
			continue
		}
		isFolder := 0
		if source.IsDir() {
			isFolder = 1
		}
		info := batchTaskInfo{FileID: source.ID(), FileName: source.Name(), IsFolder: isFolder}
		if copyType == "2" {
			info.SrcParentID = source.ParentID()
		}
		infos = append(infos, info)
	}
	if len(infos) == 0 {
		return nil
	}
	encoded, err := json.Marshal(infos)
	if err != nil {
		return err
	}
	targetID := target.ID()
	if copyType == "1" && f.isRoot(target) {
		targetID = "0"
	}
	params := url.Values{
		"type": {"COPY"}, "taskInfos": {string(encoded)}, "targetFolderId": {targetID},
		"familyId": {f.id()}, "copyType": {copyType},
	}
	taskID, err := batchTaskClient{invoker: f.base.invoker, scope: scope}.create(ctx, "COPY", params)
	if err != nil {
		return err
	}
	batch := batchTaskClient{invoker: f.base.invoker, scope: scope}
	if pollWithFamily {
		batch.pollFamily = f.id()
	}
	return batch.wait(ctx, "COPY", taskID)
}
