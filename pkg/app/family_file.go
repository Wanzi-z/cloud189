package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gowsp/cloud189/internal/invoker"
	pkg "github.com/gowsp/cloud189/pkg/drive"
)

func (f *familyAPI) Space(ctx context.Context) (pkg.Space, error) {
	if err := f.ensure(ctx); err != nil {
		return pkg.Space{}, err
	}
	var response struct {
		pkg.Space
		MaxFileSize int64 `json:"maxFilesize"`
	}
	params := url.Values{"familyId": {f.id()}}
	if err := f.base.invoker.GetScopedContext(ctx, invoker.FamilyScope, "/family/file/getUserInfo.action", params, &response); err != nil {
		return pkg.Space{}, err
	}
	f.mu.Lock()
	f.maxFileSize = response.MaxFileSize
	f.mu.Unlock()
	return response.Space, nil
}

func (f *familyAPI) List(ctx context.Context, parent pkg.Entry, fileType pkg.FileType) ([]pkg.Entry, error) {
	if err := f.ensure(ctx); err != nil {
		return nil, err
	}
	return f.list(ctx, parent, fileType, 1)
}

func (f *familyAPI) list(ctx context.Context, parent pkg.Entry, fileType pkg.FileType, page int) ([]pkg.Entry, error) {
	params := url.Values{
		"familyId":   {f.id()},
		"fileType":   {strconv.Itoa(int(fileType))},
		"mediaType":  {"0"},
		"orderBy":    {"1"},
		"descending": {"true"},
		"pageNum":    {strconv.Itoa(page)},
		"pageSize":   {"200"},
	}
	if !f.isRoot(parent) {
		params.Set("folderId", parent.ID())
	}
	var response listFileResp
	if err := f.base.invoker.GetScopedContext(ctx, invoker.FamilyScope, "/family/file/listFiles.action", params, &response); err != nil {
		return nil, err
	}
	result := make([]pkg.Entry, 0, len(response.Result.Files)+len(response.Result.Folders))
	for _, entry := range response.Result.Files {
		if entry.ParentFileID.String() == "" && !f.isRoot(parent) {
			entry.ParentFileID = json.Number(parent.ID())
		}
		result = append(result, entry)
	}
	for _, entry := range response.Result.Folders {
		result = append(result, entry)
	}
	if f.isRoot(parent) {
		for _, entry := range result {
			f.rememberRootID(entry.ParentID())
			break
		}
	}
	if page*200 < response.Result.Count {
		more, err := f.list(ctx, parent, fileType, page+1)
		if err != nil {
			return nil, err
		}
		result = append(result, more...)
	}
	return result, nil
}

func (f *familyAPI) Search(ctx context.Context, parent pkg.Entry, fileType pkg.FileType, name string) ([]pkg.Entry, error) {
	entries, err := f.List(ctx, parent, fileType)
	if err != nil {
		return nil, err
	}
	result := make([]pkg.Entry, 0, 1)
	for _, entry := range entries {
		if entry.Name() == name {
			result = append(result, entry)
		}
	}
	return result, nil
}

func (f *familyAPI) Stat(ctx context.Context, name string) (pkg.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := f.ensure(ctx); err != nil {
		return nil, err
	}
	name = path.Clean(name)
	if name == "." || name == "/" {
		return f.root, nil
	}
	current := f.root
	parts := strings.Split(strings.TrimPrefix(name, "/"), "/")
	for index, part := range parts {
		entries, err := f.List(ctx, current, pkg.All)
		if err != nil {
			return nil, err
		}
		var found pkg.Entry
		for _, entry := range entries {
			if entry.Name() == part {
				found = entry
				break
			}
		}
		if found == nil || (index < len(parts)-1 && !found.IsDir()) {
			return nil, fs.ErrNotExist
		}
		current = found
	}
	return current, nil
}

func (f *familyAPI) Mkdir(ctx context.Context, parent pkg.Entry, name string) (pkg.Entry, error) {
	if err := f.ensure(ctx); err != nil {
		return nil, err
	}
	current := parent
	parts := strings.Split(strings.Trim(path.Clean(name), "/"), "/")
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		matches, err := f.Search(ctx, current, pkg.Directory, part)
		if err != nil {
			return nil, err
		}
		if len(matches) > 0 {
			current = matches[0]
			continue
		}
		params := url.Values{"familyId": {f.id()}, "folderName": {part}}
		if !f.isRoot(current) {
			params.Set("parentId", current.ID())
		}
		var response makeDirResp
		if err := f.base.invoker.GetScopedContext(ctx, invoker.FamilyScope, "/family/file/createFolder.action", params, &response); err != nil {
			return nil, err
		}
		if err := response.Error(); err != nil {
			return nil, err
		}
		if response.Folder == nil {
			return nil, errors.New("服务端未返回新目录信息")
		}
		folder := response.Folder
		if f.isRoot(current) {
			f.rememberRootID(folder.ParentID())
		}
		current = folder
	}
	return current, nil
}

func (f *familyAPI) Rename(ctx context.Context, target pkg.Entry, name string) error {
	if target == nil {
		return fs.ErrNotExist
	}
	if target.Name() == name {
		return nil
	}
	if err := f.ensure(ctx); err != nil {
		return err
	}
	endpoint := "/family/file/renameFile.action"
	params := url.Values{"familyId": {f.id()}, "fileId": {target.ID()}, "destFileName": {name}}
	if target.IsDir() {
		endpoint = "/family/file/renameFolder.action"
		params.Del("fileId")
		params.Del("destFileName")
		params.Set("folderId", target.ID())
		params.Set("destFolderName", name)
	}
	var response map[string]any
	if err := f.base.invoker.GetScopedContext(ctx, invoker.FamilyScope, endpoint, params, &response); err != nil {
		return err
	}
	if code, ok := response["res_code"]; ok && fmt.Sprint(code) != "0" {
		return fmt.Errorf("%v: %v", code, response["res_message"])
	}
	switch entry := target.(type) {
	case *fileInfo:
		entry.FileName = name
	case *folder:
		entry.DirName = name
	}
	return nil
}

func (f *familyAPI) downloadURL(ctx context.Context, fileID string) (string, error) {
	if err := f.ensure(ctx); err != nil {
		return "", err
	}
	var response struct {
		URL string `json:"fileDownloadUrl"`
	}
	params := url.Values{"familyId": {f.id()}, "fileId": {fileID}}
	if err := f.base.invoker.GetScopedContext(ctx, invoker.FamilyScope, "/family/file/getFileDownloadUrl.action", params, &response); err != nil {
		return "", err
	}
	if response.URL == "" {
		return "", errors.New("服务端未返回下载地址")
	}
	return html.UnescapeString(response.URL), nil
}

func (f *familyAPI) Download(ctx context.Context, file pkg.Entry, start int64) (*http.Response, error) {
	if file.IsDir() {
		return nil, errors.New("not support download dir")
	}
	downloadURL, err := f.downloadURL(ctx, file.ID())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	return f.base.invoker.Send(req)
}

func (f *familyAPI) DirUsage(ctx context.Context, target pkg.Entry) (pkg.Usage, error) {
	if err := f.ensure(ctx); err != nil {
		return pkg.Usage{}, err
	}
	folderID := target.ID()
	if f.isRoot(target) {
		if folderID = f.rootID(); folderID == "" {
			entries, err := f.List(ctx, target, pkg.All)
			if err != nil {
				return pkg.Usage{}, err
			}
			if len(entries) == 0 {
				return pkg.Usage{}, nil
			}
			folderID = f.rootID()
		}
	}
	params := url.Values{"folderId": {folderID}, "familyId": {f.id()}}
	var created struct {
		ResCode    int    `json:"res_code"`
		ResMessage string `json:"res_message"`
		TaskID     string `json:"taskId"`
	}
	if err := f.base.invoker.GetScopedContext(ctx, invoker.FamilyScope, "/file/createFolderExtInfoTask.action", params, &created); err != nil {
		return pkg.Usage{}, err
	}
	if created.TaskID == "" {
		return pkg.Usage{}, errors.New("服务端未返回目录统计任务")
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		var response folderExtInfo
		query := url.Values{"taskId": {created.TaskID}}
		if err := f.base.invoker.GetScopedContext(ctx, invoker.FamilyScope, "/file/queryTaskResult.action", query, &response); err != nil {
			return pkg.Usage{}, err
		}
		switch {
		case response.TaskStatus == taskDone:
			return pkg.Usage{Files: response.FileCountNum, Directories: response.FolderCountNum, Bytes: response.FileSizeNum}, nil
		case taskPending(response.TaskStatus):
			select {
			case <-ctx.Done():
				return pkg.Usage{}, ctx.Err()
			case <-time.After(1500 * time.Millisecond):
			}
		default:
			return pkg.Usage{}, fmt.Errorf("unexpected task status: %d", response.TaskStatus)
		}
	}
	return pkg.Usage{}, errors.New("目录统计任务超时")
}
