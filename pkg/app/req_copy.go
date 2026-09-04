package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	pkg "github.com/gowsp/cloud189/pkg/drive"
)

func (c *Client) copyEntries(ctx context.Context, target pkg.Entry, files ...pkg.Entry) error {
	var errs []error
	for _, v := range files {
		err := c.copy(ctx, target, v)
		if err != nil {
			errs = append(errs, fmt.Errorf("copy %s: %w", v.Name(), err))
		}
	}
	return errors.Join(errs...)
}

func (c *Client) copy(ctx context.Context, targetFolderId pkg.Entry, src pkg.Entry) error {
	params := make(url.Values)
	params.Set("fileId", src.ID())
	params.Set("destFileName", src.Name())
	params.Set("destParentFolderId", targetFolderId.ID())
	var result map[string]interface{}
	return c.invoker.PostContext(ctx, "/copyFile.action", params, &result)
}
