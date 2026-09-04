package app

import (
	"context"
	"net/url"
	"strings"

	pkg "github.com/gowsp/cloud189/pkg/drive"
)

func (c *Client) moveEntries(ctx context.Context, target pkg.Entry, sources ...pkg.Entry) error {
	if len(sources) == 0 {
		return nil
	}
	list := make([]string, len(sources))
	for i, src := range sources {
		list[i] = src.ID()
	}
	return c.move(ctx, target.ID(), list)
}
func (c *Client) move(ctx context.Context, dir string, source []string) error {
	params := make(url.Values)
	params.Set("fileIdList", strings.Join(source, ";"))
	params.Set("destParentFolderId", dir)
	var f map[string]interface{}
	return c.invoker.PostContext(ctx, "/batchMoveFile.action", params, &f)
}
