package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/gowsp/cloud189/internal/invoker"
	pkg "github.com/gowsp/cloud189/pkg/drive"
)

type familyTaskInfo struct {
	FileID      string `json:"fileId"`
	FileName    string `json:"fileName"`
	IsFolder    int    `json:"isFolder"`
	SrcParentID string `json:"srcParentId,omitempty"`
}

type familyTaskResponse struct {
	ResCode             int           `json:"res_code"`
	ResMessage          string        `json:"res_message"`
	TaskID              string        `json:"taskId"`
	TaskStatus          int           `json:"taskStatus"`
	SubTaskCount        int           `json:"subTaskCount"`
	SuccessedCount      int           `json:"successedCount"`
	FailedCount         int           `json:"failedCount"`
	SkipCount           int           `json:"skipCount"`
	SuccessedFileIDList []json.Number `json:"successedFileIdList"`
}

func (r *familyTaskResponse) IsSuccess() bool { return r.ResCode == 0 }
func (r *familyTaskResponse) Error() string {
	return fmt.Sprintf("%d: %s", r.ResCode, r.ResMessage)
}

func (f *familyAPI) Copy(ctx context.Context, target pkg.Entry, sources ...pkg.Entry) error {
	return f.createTask(ctx, "COPY", target, sources...)
}

func (f *familyAPI) Move(ctx context.Context, target pkg.Entry, sources ...pkg.Entry) error {
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
	return f.createTask(ctx, "MOVE", target, filtered...)
}

func (f *familyAPI) Delete(ctx context.Context, sources ...pkg.Entry) error {
	return f.createTask(ctx, "DELETE", nil, sources...)
}

func (f *familyAPI) createTask(ctx context.Context, kind string, target pkg.Entry, sources ...pkg.Entry) error {
	if len(sources) == 0 {
		return nil
	}
	if err := f.ensure(ctx); err != nil {
		return err
	}
	taskInfos := make([]familyTaskInfo, 0, len(sources))
	for _, source := range sources {
		if source == nil {
			continue
		}
		isFolder := 0
		if source.IsDir() {
			isFolder = 1
		}
		taskInfos = append(taskInfos, familyTaskInfo{
			FileID: source.ID(), FileName: source.Name(), IsFolder: isFolder, SrcParentID: source.ParentID(),
		})
	}
	if len(taskInfos) == 0 {
		return nil
	}
	encoded, err := json.Marshal(taskInfos)
	if err != nil {
		return err
	}
	params := url.Values{"type": {kind}, "taskInfos": {string(encoded)}, "familyId": {f.id()}}
	if kind != "DELETE" {
		if target == nil || !target.IsDir() {
			return errors.New("target is not a directory")
		}
		targetID := target.ID()
		if f.isRoot(target) {
			targetID = "0"
		}
		params.Set("targetFolderId", targetID)
	}
	var created familyTaskResponse
	if err := f.base.invoker.PostScopedContext(ctx, invoker.FamilyScope, "/batch/createBatchTask.action", nil, params, &created); err != nil {
		return err
	}
	if created.TaskID == "" {
		return errors.New("服务端未返回批处理任务")
	}
	return f.waitTask(ctx, invoker.FamilyScope, true, kind, created.TaskID)
}

func (f *familyAPI) CopyFromPersonal(ctx context.Context, target pkg.Entry, sources ...pkg.Entry) error {
	return f.createTransferTask(ctx, "1", invoker.PersonalScope, false, target, sources...)
}

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
	infos := make([]familyTaskInfo, 0, len(sources))
	for _, source := range sources {
		if source == nil {
			continue
		}
		isFolder := 0
		if source.IsDir() {
			isFolder = 1
		}
		info := familyTaskInfo{FileID: source.ID(), FileName: source.Name(), IsFolder: isFolder}
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
	var created familyTaskResponse
	if err := f.base.invoker.PostScopedContext(ctx, scope, "/batch/createBatchTask.action", nil, params, &created); err != nil {
		return err
	}
	if created.TaskID == "" {
		return errors.New("服务端未返回转存任务")
	}
	return f.waitTask(ctx, scope, pollWithFamily, "COPY", created.TaskID)
}

func (f *familyAPI) waitTask(ctx context.Context, scope invoker.Scope, withFamily bool, kind, taskID string) error {
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		var query url.Values
		if withFamily {
			query = url.Values{"familyId": {f.id()}}
		}
		params := url.Values{"taskId": {taskID}, "type": {kind}}
		var response familyTaskResponse
		if err := f.base.invoker.PostScopedContext(ctx, scope, "/batch/checkBatchTask.action", query, params, &response); err != nil {
			return err
		}
		switch response.TaskStatus {
		case 2:
			return fmt.Errorf("目标目录存在同名文件: 成功 %d, 失败 %d, 跳过 %d", response.SuccessedCount, response.FailedCount, response.SkipCount)
		case 4:
			if response.FailedCount > 0 {
				return fmt.Errorf("批处理部分失败: 成功 %d, 失败 %d, 跳过 %d", response.SuccessedCount, response.FailedCount, response.SkipCount)
			}
			return nil
		case 3:
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1500 * time.Millisecond):
			}
		default:
			return fmt.Errorf("unexpected task status: %s", strconv.Itoa(response.TaskStatus))
		}
	}
	return fmt.Errorf("批处理任务超时: %s", taskID)
}
