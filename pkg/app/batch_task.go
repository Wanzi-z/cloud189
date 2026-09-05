package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/gowsp/cloud189/internal/invoker"
	pkg "github.com/gowsp/cloud189/pkg/drive"
)

type batchTaskInfo struct {
	FileID      string `json:"fileId"`
	FileName    string `json:"fileName"`
	IsFolder    int    `json:"isFolder"`
	SrcParentID string `json:"srcParentId,omitempty"`
}

type batchTaskResponse struct {
	ResCode             int           `json:"res_code"`
	ResMessage          string        `json:"res_message"`
	TaskID              string        `json:"taskId"`
	TaskStatus          int           `json:"taskStatus"`
	SubTaskCount        int           `json:"subTaskCount"`
	SucceededCount      int           `json:"successedCount"`
	FailedCount         int           `json:"failedCount"`
	SkipCount           int           `json:"skipCount"`
	SuccessedFileIDList []json.Number `json:"successedFileIdList"`
}

const (
	taskQueued   = 1
	taskConflict = 2
	taskRunning  = 3
	taskDone     = 4
)

// Conflict resolution strategies applied when a batch task reports
// taskStatus=2 (duplicate name in target directory).
//
// dealWay=1 skips the conflicting entry: the existing target is kept.
// dealWay=2 keeps both entries: the incoming copy is auto-renamed.
// dealWay=3 overwrites: the existing target entry is replaced.
const (
	dealWaySkip      = 1
	dealWayKeepBoth  = 2
	dealWayOverwrite = 3
)

// ConflictPolicy decides how a conflict task is resolved via
// /batch/manageBatchTask.action.
type ConflictPolicy int

const (
	// ConflictError aborts with an error when duplicates are detected.
	ConflictError ConflictPolicy = iota
	// ConflictSkip resolves duplicates by keeping the existing target file.
	ConflictSkip
	// ConflictKeepBoth resolves duplicates by keeping both files.
	ConflictKeepBoth
	// ConflictOverwrite resolves duplicates by overwriting the target.
	ConflictOverwrite
)

func taskPending(status int) bool {
	return status == taskQueued || status == taskRunning
}

func (r batchTaskResponse) resultError() error {
	if r.FailedCount > 0 || r.SkipCount > 0 {
		return fmt.Errorf("批处理未全部成功: 成功 %d, 失败 %d, 跳过 %d", r.SucceededCount, r.FailedCount, r.SkipCount)
	}
	if r.SubTaskCount > 0 && r.SucceededCount != r.SubTaskCount {
		return fmt.Errorf("批处理结果不完整: 子任务 %d, 成功 %d", r.SubTaskCount, r.SucceededCount)
	}
	return nil
}

func (r *batchTaskResponse) IsSuccess() bool { return r.ResCode == 0 }
func (r *batchTaskResponse) Error() string {
	return fmt.Sprintf("%d: %s", r.ResCode, r.ResMessage)
}

type taskObserverKey struct{}

func withTaskObserver(ctx context.Context, observer func(batchTaskResponse)) context.Context {
	return context.WithValue(ctx, taskObserverKey{}, observer)
}

func observeTask(ctx context.Context, response batchTaskResponse) {
	if observer, ok := ctx.Value(taskObserverKey{}).(func(batchTaskResponse)); ok {
		observer(response)
	}
}

type batchTaskClient struct {
	invoker    *invoker.Invoker
	scope      invoker.Scope
	pollFamily string
	conflict   ConflictPolicy
}

func (c *Client) personalBatch() batchTaskClient {
	return batchTaskClient{invoker: c.invoker, scope: invoker.PersonalScope}
}

func (f *familyAPI) familyBatch() batchTaskClient {
	return batchTaskClient{invoker: f.base.invoker, scope: invoker.FamilyScope, pollFamily: f.id()}
}

func (b batchTaskClient) create(ctx context.Context, kind string, params url.Values) (string, error) {
	var created batchTaskResponse
	if err := b.invoker.PostScopedContext(ctx, b.scope, "/batch/createBatchTask.action", nil, params, &created); err != nil {
		return "", err
	}
	if created.TaskID == "" {
		return "", errors.New("服务端未返回批处理任务")
	}
	return created.TaskID, nil
}

func (b batchTaskClient) wait(ctx context.Context, kind, taskID string) error {
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		var query url.Values
		if b.pollFamily != "" {
			query = url.Values{"familyId": {b.pollFamily}}
		}
		params := url.Values{"taskId": {taskID}, "type": {kind}}
		var response batchTaskResponse
		if err := b.invoker.PostScopedContext(ctx, b.scope, "/batch/checkBatchTask.action", query, params, &response); err != nil {
			return err
		}
		observeTask(ctx, response)
		switch {
		case response.TaskStatus == taskConflict:
			if b.conflict == ConflictError {
				return fmt.Errorf("目标目录存在同名文件: 成功 %d, 失败 %d, 跳过 %d", response.SucceededCount, response.FailedCount, response.SkipCount)
			}
			if err := b.resolveConflict(ctx, kind, taskID); err != nil {
				return err
			}
		case response.TaskStatus == taskDone:
			if b.conflict == ConflictSkip {
				// dealWay=1 reports skipped entries as success on the server
				// (successedCount=0, skipCount=1, taskStatus=4).
				return nil
			}
			return response.resultError()
		case taskPending(response.TaskStatus):
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1500 * time.Millisecond):
			}
		default:
			return fmt.Errorf("unexpected task status: %d", response.TaskStatus)
		}
	}
	return fmt.Errorf("批处理任务超时: %s", taskID)
}

type conflictInfoResp struct {
	ResCode    int    `json:"res_code"`
	ResMessage string `json:"res_message"`
	TaskID     string `json:"taskId"`
	TaskInfos  []struct {
		FileID   json.Number `json:"fileId"`
		FileName string      `json:"fileName"`
		IsFolder int         `json:"isFolder"`
	} `json:"taskInfos"`
}

// resolveConflict implements the desktop flow captured in 8_Full.txt:
// getConflictTaskInfo lists conflicting entries, then manageBatchTask
// applies the chosen dealWay to each and the task completes afterwards.
func (b batchTaskClient) resolveConflict(ctx context.Context, kind, taskID string) error {
	var query url.Values
	if b.pollFamily != "" {
		query = url.Values{"familyId": {b.pollFamily}}
	}
	var conflicts conflictInfoResp
	info := url.Values{"taskId": {taskID}, "type": {kind}}
	if err := b.invoker.PostScopedContext(ctx, b.scope, "/batch/getConflictTaskInfo.action", query, info, &conflicts); err != nil {
		return err
	}
	if len(conflicts.TaskInfos) == 0 {
		return errors.New("服务端未返回冲突文件信息")
	}
	dealWay := dealWayKeepBoth
	switch b.conflict {
	case ConflictSkip:
		dealWay = dealWaySkip
	case ConflictOverwrite:
		dealWay = dealWayOverwrite
	}
	// manageBatchTask requires dealWay inline per taskInfo.
	payload := make([]map[string]any, 0, len(conflicts.TaskInfos))
	for _, item := range conflicts.TaskInfos {
		payload = append(payload, map[string]any{
			"fileId": item.FileID.String(), "fileName": item.FileName,
			"isFolder": item.IsFolder, "dealWay": dealWay,
		})
	}
	managedJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	params := url.Values{"taskId": {taskID}, "type": {kind}, "taskInfos": {string(managedJSON)}}
	var result struct {
		ResCode    int    `json:"res_code"`
		ResMessage string `json:"res_message"`
		Success    bool   `json:"success"`
	}
	return b.invoker.PostScopedContext(ctx, b.scope, "/batch/manageBatchTask.action", query, params, &result)
}

func (b batchTaskClient) run(ctx context.Context, kind string, target *string, sources ...pkg.Entry) error {
	if len(sources) == 0 {
		return nil
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
		infos = append(infos, batchTaskInfo{
			FileID: source.ID(), FileName: source.Name(), IsFolder: isFolder, SrcParentID: source.ParentID(),
		})
	}
	if len(infos) == 0 {
		return nil
	}
	encoded, err := json.Marshal(infos)
	if err != nil {
		return err
	}
	params := url.Values{"type": {kind}, "taskInfos": {string(encoded)}}
	if kind != "DELETE" {
		if target == nil {
			return errors.New("target is not a directory")
		}
		params.Set("targetFolderId", *target)
	}
	if b.pollFamily != "" {
		params.Set("familyId", b.pollFamily)
	}
	taskID, err := b.create(ctx, kind, params)
	if err != nil {
		return err
	}
	return b.wait(ctx, kind, taskID)
}

func (c *Client) Copy(ctx context.Context, target pkg.Entry, sources ...pkg.Entry) error {
	return c.personalBatch().run(ctx, "COPY", targetID(target), sources...)
}

func (c *Client) Move(ctx context.Context, target pkg.Entry, sources ...pkg.Entry) error {
	return c.personalBatch().run(ctx, "MOVE", targetID(target), sources...)
}

func (c *Client) CopyPolicy(ctx context.Context, policy pkg.ConflictPolicy, target pkg.Entry, sources ...pkg.Entry) error {
	batch := c.personalBatch()
	batch.conflict = toConflictPolicy(policy)
	return batch.run(ctx, "COPY", targetID(target), sources...)
}

func (c *Client) MovePolicy(ctx context.Context, policy pkg.ConflictPolicy, target pkg.Entry, sources ...pkg.Entry) error {
	batch := c.personalBatch()
	batch.conflict = toConflictPolicy(policy)
	return batch.run(ctx, "MOVE", targetID(target), sources...)
}

func toConflictPolicy(policy pkg.ConflictPolicy) ConflictPolicy {
	switch policy {
	case pkg.ConflictSkip:
		return ConflictSkip
	case pkg.ConflictKeepBoth:
		return ConflictKeepBoth
	case pkg.ConflictOverwrite:
		return ConflictOverwrite
	default:
		return ConflictError
	}
}

func (c *Client) Remove(ctx context.Context, sources ...pkg.Entry) error {
	return c.personalBatch().run(ctx, "DELETE", nil, sources...)
}

func targetID(target pkg.Entry) *string {
	if target == nil || !target.IsDir() {
		return nil
	}
	id := target.ID()
	return &id
}
