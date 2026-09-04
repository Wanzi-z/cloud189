package app

import (
	"context"
	"net/url"
	"strings"

	pkg "github.com/gowsp/cloud189/pkg/drive"
)

func (c *Client) remove(ctx context.Context, files ...pkg.Entry) error {
	if len(files) == 0 {
		return nil
	}
	list := make([]string, len(files))
	for i, src := range files {
		list[i] = src.ID()
	}
	return c.deleteFile(ctx, list)
}
func (c *Client) deleteFile(ctx context.Context, list []string) error {
	params := make(url.Values)
	id := strings.Join(list, ";")
	params.Set("fileIdList", id)
	var f map[string]interface{}
	return c.invoker.PostContext(ctx, "/batchDeleteFile.action", params, &f)
}
