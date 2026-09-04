package app

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	pkg "github.com/gowsp/cloud189/pkg/drive"
)

func (d *Client) searchEntries(ctx context.Context, parent pkg.Entry, fileType pkg.FileType, name string) ([]pkg.Entry, error) {
	return d.search(ctx, parent.ID(), strconv.Itoa(int(fileType)), name, 1)
}

type searchResult struct {
	Code    int         `json:"res_code"`
	Message string      `json:"res_message"`
	Count   int         `json:"count"`
	Files   []*fileInfo `json:"fileList"`
	Folders []*folder   `json:"folderList"`
}

func (l *searchResult) fill(id string) (data []pkg.Entry) {
	if l == nil || l.Count == 0 {
		return
	}
	for _, f := range l.Files {
		f.ParentFileID = json.Number(id)
		data = append(data, f)
	}
	for _, f := range l.Folders {
		data = append(data, f)
	}
	return
}

func (c *Client) search(ctx context.Context, id, fileType, name string, page int) (result []pkg.Entry, err error) {
	if isSystemFolder(id, name) {
		return c.listEntries(ctx, personalRoot, pkg.Directory)
	}
	params := make(url.Values)
	params.Set("folderId", id)
	params.Set("filename", name)
	params.Set("fileType", fileType)
	params.Set("mediaType", "0")
	params.Set("mediaAttr", "0")
	params.Set("recursive", "0")
	params.Set("iconOption", "0")
	params.Set("descending", "true")
	params.Set("orderBy", "filename")
	params.Set("pageNum", strconv.Itoa(page))
	params.Set("pageSize", "100")
	var files searchResult
	err = c.invoker.GetContext(ctx, "/searchFiles.action", params, &files)
	if err != nil {
		return
	}
	result = append(result, files.fill(id)...)
	if page*100 < files.Count {
		var more []pkg.Entry
		more, err = c.search(ctx, id, fileType, name, page+1)
		result = append(result, more...)
	}
	return
}
