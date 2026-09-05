package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gowsp/cloud189/internal/invoker"
	pkg "github.com/gowsp/cloud189/pkg/drive"
)

func (c *Client) detail(ctx context.Context, id string) (string, error) {
	var info struct {
		URL string `json:"fileDownloadUrl"`
	}
	err := c.invoker.GetContext(ctx, "/getFileDownloadUrl.action", url.Values{"fileId": {id}}, &info)
	return info.URL, err
}

func (c *Client) download(ctx context.Context, file pkg.Entry, start int64) (*http.Response, error) {
	if file.IsDir() {
		return nil, errors.New("not support download dir")
	}
	url, err := c.detail(ctx, file.ID())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	return c.invoker.Send(req)
}

type simpleFolder struct {
	Fid   int    `json:"fid"`
	Fname string `json:"fname"`
}
type fileDetail struct {
	*invoker.NumCodeRsp
	CreateDate      string `json:"createDate"`
	FileDownloadURL string `json:"fileDownloadUrl"`
	FilePath        string `json:"filePath"`
	Icon            struct {
		LargeURL  string `json:"largeUrl"`
		MediumURL string `json:"mediumUrl"`
		SmallURL  string `json:"smallUrl"`
	} `json:"icon"`
	FileID             int64  `json:"id"`
	LastOpTime         int64  `json:"lastOpTime"`
	LastOpTimeStr      string `json:"lastOpTimeStr"`
	Md5                string `json:"md5"`
	MediaType          int    `json:"mediaType"`
	FileName           string `json:"name"`
	ParentFolderListAO struct {
		ParentFolderList []simpleFolder `json:"parentFolderList"`
	} `json:"parentFolderListAO"`
	ParentFileID int64 `json:"parentId"`
	Rev          int64 `json:"rev"`
	FileSize     int64 `json:"size"`
}

func (f *fileDetail) ID() string       { return fmt.Sprintf("%d", f.FileID) }
func (f *fileDetail) ParentID() string { return fmt.Sprintf("%d", f.ParentFileID) }
func (f *fileDetail) Name() string     { return f.FileName }
func (f *fileDetail) Size() int64      { return f.FileSize }
func (f *fileDetail) Mode() os.FileMode {
	if f.IsDir() {
		return os.ModeDir | 0555
	}
	return 0444
}
func (f *fileDetail) ModTime() time.Time         { return unixTime(f.LastOpTime) }
func (f *fileDetail) IsDir() bool                { return f.Md5 == "" }
func (f *fileDetail) Sys() any                   { return f.ParentFolderListAO.ParentFolderList }
func (f *fileDetail) Type() os.FileMode          { return f.Mode().Type() }
func (f *fileDetail) Info() (os.FileInfo, error) { return f, nil }

func unixTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value)
	}
	return time.Unix(value, 0)
}

type folderInfo struct {
	CreateDate         string `json:"createDate"`
	CreateTime         int64  `json:"createTime"`
	FileID             int64  `json:"fileId"`
	FileName           string `json:"fileName"`
	FilePath           string `json:"filePath"`
	LastOpTime         int64  `json:"lastOpTime"`
	LastOpTimeStr      string `json:"lastOpTimeStr"`
	ParentFolderListAO struct {
		ParentFolderList []simpleFolder `json:"parentFolderList"`
	} `json:"parentFolderListAO"`
	ParentID int64 `json:"parentId"`
	Rev      int64 `json:"rev"`
}

type folderExtInfo struct {
	*invoker.NumCodeRsp
	FileCountNum   uint64 `json:"fileCount"`
	FileSizeNum    uint64 `json:"fileSize"`
	FolderCountNum uint64 `json:"folderCount"`
	FolderID       int64  `json:"folderId"`
	RecursionFlag  int    `json:"recursionFlag"`
	TaskID         string `json:"taskId"`
	TaskStatus     int    `json:"taskStatus"`
}

func (f *folderExtInfo) FileCount() uint64   { return f.FileCountNum }
func (f *folderExtInfo) FileSize() uint64    { return f.FileSizeNum }
func (f *folderExtInfo) FolderCount() uint64 { return f.FolderCountNum }

func (c *Client) usage(ctx context.Context, file pkg.Entry) (pkg.Usage, error) {
	response := &struct {
		ResCode    int    `json:"res_code"`
		ResMessage string `json:"res_message"`
		TaskId     string `json:"taskId"`
	}{}
	err := c.invoker.GetContext(ctx, "/file/createFolderExtInfoTask.action", url.Values{"folderId": {file.ID()}}, &response)
	if err != nil {
		return pkg.Usage{}, err
	}
	rsp := new(folderExtInfo)
	req := url.Values{"taskId": {response.TaskId}}
	// 循环查询任务结果，直到状态变为4（完成）或出现错误
	for {
		err = c.invoker.GetContext(ctx, "/file/queryTaskResult.action", req, rsp)
		if err != nil {
			return pkg.Usage{}, err
		}
		// 如果任务完成则返回结果
		if rsp.TaskStatus == taskDone {
			return pkg.Usage{Files: rsp.FileCountNum, Directories: rsp.FolderCountNum, Bytes: rsp.FileSizeNum}, nil
		}
		// 如果不是状态3（进行中），则返回错误
		if !taskPending(rsp.TaskStatus) {
			return pkg.Usage{}, fmt.Errorf("unexpected task status: %d", rsp.TaskStatus)
		}
		// 等待1.5秒再重试
		select {
		case <-ctx.Done():
			return pkg.Usage{}, ctx.Err()
		case <-time.After(1500 * time.Millisecond):
		}
	}
}
func (c *Client) getFolderInfoByID(id string) (*folderInfo, error) {
	response := &struct {
		ResCode    int    `json:"res_code"`
		ResMessage string `json:"res_message"`
		folderInfo
	}{}
	err := c.invoker.Get("/getFolderInfo.action", url.Values{
		"folderId":   {id},
		"folderPath": {},
		"pathList":   {"1"},
		"dt":         {"3"},
	}, &response)
	return &response.folderInfo, err
}
func (c *Client) getFolderInfoByPath(path string) (*folderInfo, error) {
	response := &struct {
		ResCode    int    `json:"res_code"`
		ResMessage string `json:"res_message"`
		folderInfo
	}{}
	err := c.invoker.Get("/getFolderInfo.action", url.Values{
		"folderId":   {},
		"folderPath": {path},
		"pathList":   {"1"},
		"dt":         {"3"},
	}, &response)
	return &response.folderInfo, err
}

func (c *Client) stat(ctx context.Context, path string) (pkg.Entry, error) {
	response := new(fileDetail)
	err := c.invoker.GetContext(ctx, "/getFileInfo.action", url.Values{
		"fileId":     {},
		"filePath":   {path},
		"pathList":   {"1"},
		"iconOption": {"0"},
	}, response)
	if rsp, ok := err.(invoker.BadRsp); ok {
		if rsp.IsError(invoker.ErrFileNotFound) {
			return nil, os.ErrNotExist
		}
	}
	return response, err
}
func (c *Client) getFileInfoByID(id string) (*fileDetail, error) {
	response := &struct {
		ResCode    int    `json:"res_code"`
		ResMessage string `json:"res_message"`
		fileDetail
	}{}
	err := c.invoker.Get("/getFileInfo.action", url.Values{
		"fileId":     {id},
		"filePath":   {},
		"pathList":   {"1"},
		"iconOption": {"0"},
	}, &response)
	return &response.fileDetail, err
}
