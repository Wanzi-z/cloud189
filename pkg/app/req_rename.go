package app

import (
	"context"
	"net/url"
	"os"

	pkg "github.com/gowsp/cloud189/pkg/drive"
)

func (c *Client) rename(ctx context.Context, src pkg.Entry, dest string) (err error) {
	if src == nil {
		return os.ErrNotExist
	}
	if src.Name() == dest {
		return nil
	}
	if src.IsDir() {
		err = c.renameFoler(ctx, src.ID(), dest)
	} else {
		err = c.renameFile(ctx, src.ID(), dest)
	}
	return
}
func (c *Client) renameFile(ctx context.Context, id, dest string) error {
	params := make(url.Values)
	params.Set("fileId", id)
	params.Set("destFileName", dest)
	var f map[string]interface{}
	return c.invoker.PostContext(ctx, "/renameFile.action", params, &f)
}
func (c *Client) renameFoler(ctx context.Context, id, dest string) error {
	params := make(url.Values)
	params.Set("folderId", id)
	params.Set("destFolderName", dest)
	var result map[string]interface{}
	return c.invoker.PostContext(ctx, "/renameFolder.action", params, &result)
}
