package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	appupload "github.com/gowsp/cloud189/internal/app/upload"
	"github.com/gowsp/cloud189/pkg/drive"
)

type personalBackend struct{ client *Client }
type familyBackend struct{ family *familyAPI }

func (c *Client) Personal() *drive.FS {
	return drive.New(&personalBackend{client: c})
}

func (c *Client) Family(ctx context.Context, familyID string) (*drive.FS, error) {
	family := &familyAPI{base: c, selector: familyID, root: newFamilyRoot(familyID)}
	if err := family.ensure(ctx); err != nil {
		return nil, err
	}
	return drive.New(&familyBackend{family: family}), nil
}

func (*personalBackend) Root() drive.Entry { return personalRoot }
func (b *familyBackend) Root() drive.Entry { return b.family.root }

func (b *personalBackend) Stat(ctx context.Context, name string) (drive.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if name == "." {
		return personalRoot, nil
	}
	return b.client.stat(ctx, "/"+name)
}
func (b *familyBackend) Stat(ctx context.Context, name string) (drive.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if name == "." {
		return b.family.root, nil
	}
	return b.family.Stat(ctx, "/"+name)
}

func (b *personalBackend) List(ctx context.Context, parent drive.Entry, kind drive.FileType) ([]drive.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b.client.listEntries(ctx, parent, kind)
}
func (b *familyBackend) List(ctx context.Context, parent drive.Entry, kind drive.FileType) ([]drive.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b.family.List(ctx, parent, kind)
}

func (b *personalBackend) Mkdir(ctx context.Context, parent drive.Entry, name string) (drive.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b.client.mkdir(ctx, parent, name)
}
func (b *familyBackend) Mkdir(ctx context.Context, parent drive.Entry, name string) (drive.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b.family.Mkdir(ctx, parent, name)
}

func (b *personalBackend) Rename(ctx context.Context, entry drive.Entry, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.client.rename(ctx, entry, name)
}
func (b *familyBackend) Rename(ctx context.Context, entry drive.Entry, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.family.Rename(ctx, entry, name)
}
func (b *personalBackend) Move(ctx context.Context, target drive.Entry, entries ...drive.Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.client.moveEntries(ctx, target, entries...)
}
func (b *familyBackend) Move(ctx context.Context, target drive.Entry, entries ...drive.Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.family.Move(ctx, target, entries...)
}
func (b *personalBackend) Copy(ctx context.Context, target drive.Entry, entries ...drive.Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.client.copyEntries(ctx, target, entries...)
}
func (b *familyBackend) Copy(ctx context.Context, target drive.Entry, entries ...drive.Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.family.Copy(ctx, target, entries...)
}
func (b *personalBackend) Remove(ctx context.Context, entries ...drive.Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.client.remove(ctx, entries...)
}
func (b *familyBackend) Remove(ctx context.Context, entries ...drive.Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.family.Delete(ctx, entries...)
}

func (b *personalBackend) Space(ctx context.Context) (drive.Space, error) {
	if err := ctx.Err(); err != nil {
		return drive.Space{}, err
	}
	return b.client.space(ctx)
}
func (b *familyBackend) Space(ctx context.Context) (drive.Space, error) {
	if err := ctx.Err(); err != nil {
		return drive.Space{}, err
	}
	return b.family.Space(ctx)
}
func (b *personalBackend) Usage(ctx context.Context, entry drive.Entry) (drive.Usage, error) {
	if err := ctx.Err(); err != nil {
		return drive.Usage{}, err
	}
	return b.client.usage(ctx, entry)
}
func (b *familyBackend) Usage(ctx context.Context, entry drive.Entry) (drive.Usage, error) {
	if err := ctx.Err(); err != nil {
		return drive.Usage{}, err
	}
	return b.family.DirUsage(ctx, entry)
}

func openResponse(resp *http.Response, err error) (io.ReadCloser, error) {
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		resp.Body.Close()
		return nil, fmt.Errorf("download returned %s", resp.Status)
	}
	return resp.Body, nil
}
func (b *personalBackend) Open(ctx context.Context, entry drive.Entry, offset int64) (io.ReadCloser, error) {
	return openResponse(b.client.download(ctx, entry, offset))
}
func (b *familyBackend) Open(ctx context.Context, entry drive.Entry, offset int64) (io.ReadCloser, error) {
	return openResponse(b.family.Download(ctx, entry, offset))
}
func (b *personalBackend) DownloadURL(ctx context.Context, entry drive.Entry) (*url.URL, error) {
	raw, err := b.client.detail(ctx, entry.ID())
	if err != nil {
		return nil, err
	}
	return url.Parse(raw)
}
func (b *familyBackend) DownloadURL(ctx context.Context, entry drive.Entry) (*url.URL, error) {
	raw, err := b.family.downloadURL(ctx, entry.ID())
	if err != nil {
		return nil, err
	}
	return url.Parse(raw)
}

func makeTempSource(ctx context.Context, r io.Reader) (string, error) {
	tmp, err := os.CreateTemp("", "cloud189-put-*")
	if err != nil {
		return "", err
	}
	path := tmp.Name()
	_, copyErr := io.Copy(tmp, &contextReader{ctx: ctx, reader: r})
	err = errors.Join(copyErr, tmp.Close())
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (b *personalBackend) Put(ctx context.Context, parent drive.Entry, name string, r io.Reader, size int64, options drive.PutOptions) error {
	return writeReader(ctx, b.client.uploader(ctx), parent, name, r, size, options)
}
func (b *familyBackend) Put(ctx context.Context, parent drive.Entry, name string, r io.Reader, size int64, options drive.PutOptions) error {
	return writeReader(ctx, b.family.uploader(ctx), parent, name, r, size, options)
}

func (b *personalBackend) PutDigest(ctx context.Context, parent drive.Entry, name string, size int64, digest drive.Digest, options drive.PutOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeDigest(b.client.uploader(ctx), parent, name, size, digest, options)
}
func (b *familyBackend) PutDigest(ctx context.Context, parent drive.Entry, name string, size int64, digest drive.Digest, options drive.PutOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeDigest(b.family.uploader(ctx), parent, name, size, digest, options)
}

func (b *familyBackend) FamilyID(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return b.family.FamilyID(ctx)
}
func (b *familyBackend) CopyFromPersonal(ctx context.Context, target drive.Entry, entries ...drive.Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.family.CopyFromPersonal(ctx, target, entries...)
}
func (b *familyBackend) CopyToPersonal(ctx context.Context, target drive.Entry, entries ...drive.Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.family.CopyToPersonal(ctx, target, entries...)
}

func writeReader(ctx context.Context, writer appupload.Writer, parent drive.Entry, name string, r io.Reader, size int64, options drive.PutOptions) error {
	path, err := makeTempSource(ctx, r)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	source, err := appupload.NewFile(parent.ID(), path, name, options.Overwrite)
	if err != nil {
		return err
	}
	if size >= 0 && source.Size() != size {
		_ = source.Close()
		return fmt.Errorf("upload size mismatch: got %d, want %d", source.Size(), size)
	}
	return errors.Join(writer.Write(source), source.Close())
}

func writeDigest(writer appupload.Writer, parent drive.Entry, name string, size int64, digest drive.Digest, options drive.PutOptions) error {
	source, err := appupload.NewDigest(parent.ID(), name, size, digest.MD5, digest.SliceMD5, options.Overwrite)
	if err != nil {
		return err
	}
	return errors.Join(writer.Write(source), source.Close())
}
