package app

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	pkg "github.com/gowsp/cloud189/pkg/drive"
)

func (d *Client) listEntries(ctx context.Context, parent pkg.Entry, fileType pkg.FileType) ([]pkg.Entry, error) {
	id := parent.ID()
	return d.list(ctx, id, strconv.Itoa(int(fileType)), 1)
}

type listFileResp struct {
	Code    int    `json:"res_code"`
	Message string `json:"res_message"`
	Result  struct {
		Count int `json:"count"`
		Size  int `json:"fileListSize"`

		Files   []*fileInfo `json:"fileList"`
		Folders []*folder   `json:"folderList"`
	} `json:"fileListAO"`
	LastRev int64 `json:"lastRev"`
}

func (l *listFileResp) fill(id string) (data []pkg.Entry) {
	if l == nil || l.Result.Count == 0 {
		return
	}
	for _, f := range l.Result.Files {
		f.ParentFileID = json.Number(id)
		data = append(data, f)
	}
	for _, f := range l.Result.Folders {
		data = append(data, f)
	}
	return
}

func (c *Client) list(ctx context.Context, id, fileType string, page int) (result []pkg.Entry, err error) {
	params := make(url.Values)
	params.Set("folderId", id)
	params.Set("fileType", fileType)
	params.Set("mediaType", "0")
	params.Set("mediaAttr", "0")
	params.Set("iconOption", "0")
	params.Set("orderBy", "filename")
	params.Set("descending", "true")
	params.Set("pageNum", strconv.Itoa(page))
	params.Set("pageSize", "100")

	var resp listFileResp
	err = c.invoker.GetContext(ctx, "/listFiles.action", params, &resp)
	if err != nil {
		return
	}
	result = append(result, resp.fill(id)...)
	if 100*page < resp.Result.Count {
		var more []pkg.Entry
		more, err = c.list(ctx, id, fileType, page+1)
		result = append(result, more...)
	}
	return
}
