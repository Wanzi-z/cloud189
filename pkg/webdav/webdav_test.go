package webdav

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/gowsp/cloud189/pkg/drive"
)

type davFile struct {
	id, parent, name, data string
	dir                    bool
}

func (f *davFile) ID() string       { return f.id }
func (f *davFile) ParentID() string { return f.parent }
func (f *davFile) Name() string     { return f.name }
func (f *davFile) Size() int64      { return 0 }
func (f *davFile) Mode() fs.FileMode {
	if f.dir {
		return fs.ModeDir | 0555
	}
	return 0444
}
func (f *davFile) ModTime() time.Time         { return time.Time{} }
func (f *davFile) IsDir() bool                { return f.dir }
func (f *davFile) Sys() any                   { return nil }
func (f *davFile) Type() fs.FileMode          { return f.Mode().Type() }
func (f *davFile) Info() (fs.FileInfo, error) { return f, nil }

type davBackend struct{ files map[string]*davFile }

func newDAVBackend() *davBackend {
	return &davBackend{files: map[string]*davFile{"source.txt": {id: "source", parent: "root", name: "source.txt", data: "source"}}}
}
func (*davBackend) Root() drive.Entry                          { return &davFile{id: "root", name: ".", dir: true} }
func (*davBackend) Space(context.Context) (drive.Space, error) { return drive.Space{}, nil }
func (*davBackend) Usage(context.Context, drive.Entry) (drive.Usage, error) {
	return drive.Usage{}, nil
}
func (*davBackend) Rename(context.Context, drive.Entry, string) error       { return nil }
func (*davBackend) Move(context.Context, drive.Entry, ...drive.Entry) error { return nil }
func (b *davBackend) Remove(_ context.Context, entries ...drive.Entry) error {
	for key, item := range b.files {
		for _, entry := range entries {
			if item == entry {
				delete(b.files, key)
			}
		}
	}
	return nil
}
func (*davBackend) Open(_ context.Context, entry drive.Entry, _ int64) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(entry.(*davFile).data)), nil
}
func (*davBackend) DownloadURL(context.Context, drive.Entry) (*url.URL, error) {
	return nil, fs.ErrInvalid
}

func (b *davBackend) Put(_ context.Context, parent drive.Entry, name string, reader io.Reader, _ int64, _ drive.PutOptions) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	key := name
	if parent.Name() != "." {
		key = path.Join(parent.Name(), name)
	}
	b.files[key] = &davFile{id: key, parent: parent.ID(), name: path.Base(name), data: string(data)}
	return nil
}
func (*davBackend) PutDigest(context.Context, drive.Entry, string, int64, drive.Digest, drive.PutOptions) error {
	return nil
}

func (b *davBackend) Stat(_ context.Context, name string) (drive.Entry, error) {
	if item := b.files[name]; item != nil {
		return item, nil
	}
	return nil, fs.ErrNotExist
}
func (b *davBackend) List(_ context.Context, parent drive.Entry, _ drive.FileType) ([]drive.Entry, error) {
	result := []drive.Entry{}
	for _, item := range b.files {
		if item.parent == parent.ID() {
			result = append(result, item)
		}
	}
	return result, nil
}
func (b *davBackend) Mkdir(_ context.Context, parent drive.Entry, name string) (drive.Entry, error) {
	item := &davFile{id: name, parent: parent.ID(), name: name, dir: true}
	b.files[name] = item
	return item, nil
}
func (b *davBackend) Copy(_ context.Context, target drive.Entry, entries ...drive.Entry) error {
	for _, entry := range entries {
		key := entry.Name()
		if target.Name() != "." {
			key = target.Name() + "/" + key
		}
		b.files[key] = &davFile{id: key, parent: target.ID(), name: entry.Name()}
	}
	return nil
}
func (b *davBackend) CopyWithOptions(ctx context.Context, target drive.Entry, _ drive.ConflictPolicy, entries ...drive.Entry) error {
	return b.Copy(ctx, target, entries...)
}
func (b *davBackend) MoveWithOptions(ctx context.Context, target drive.Entry, _ drive.ConflictPolicy, entries ...drive.Entry) error {
	return b.Move(ctx, target, entries...)
}

func TestFileSystemUsesWebDAVPaths(t *testing.T) {
	adapter := NewFileSystem(drive.New(newDAVBackend()))
	if info, err := adapter.Stat(context.Background(), "/"); err != nil || info.Name() != "." {
		t.Fatalf("Stat(/) = %v, %v", info, err)
	}
	if _, err := adapter.Stat(context.Background(), "relative"); err == nil {
		t.Fatal("relative WebDAV path was accepted")
	}
	if _, err := adapter.Stat(context.Background(), "/../secret"); err == nil {
		t.Fatal("traversal WebDAV path was accepted")
	}
}

func TestHandlerBasics(t *testing.T) {
	backend := newDAVBackend()
	handler := NewHandler("", drive.New(backend))

	mkdir := httptest.NewRequest("MKCOL", "/new", nil)
	mkdirResult := httptest.NewRecorder()
	handler.ServeHTTP(mkdirResult, mkdir)
	if mkdirResult.Code != http.StatusCreated {
		t.Fatalf("MKCOL status = %d, body = %s", mkdirResult.Code, mkdirResult.Body.String())
	}

	propfind := httptest.NewRequest("PROPFIND", "/new", strings.NewReader(""))
	propfind.Header.Set("Depth", "0")
	propfindResult := httptest.NewRecorder()
	handler.ServeHTTP(propfindResult, propfind)
	if propfindResult.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND status = %d, body = %s", propfindResult.Code, propfindResult.Body.String())
	}

	put := httptest.NewRequest(http.MethodPut, "/uploaded.txt", strings.NewReader("data"))
	putResult := httptest.NewRecorder()
	handler.ServeHTTP(putResult, put)
	if putResult.Code != http.StatusCreated {
		t.Fatalf("PUT status = %d, body = %s", putResult.Code, putResult.Body.String())
	}

	copyRequest := httptest.NewRequest("COPY", "/source.txt", nil)
	copyRequest.Header.Set("Destination", "/new/source.txt")
	copyResult := httptest.NewRecorder()
	handler.ServeHTTP(copyResult, copyRequest)
	if copyResult.Code != http.StatusCreated {
		t.Fatalf("COPY status = %d, body = %s", copyResult.Code, copyResult.Body.String())
	}

	copyResult = httptest.NewRecorder()
	handler.ServeHTTP(copyResult, copyRequest)
	if copyResult.Code != http.StatusNoContent {
		t.Fatalf("overwriting COPY status = %d, body = %s", copyResult.Code, copyResult.Body.String())
	}
}

func TestFileSystemCreateWriteClose(t *testing.T) {
	backend := newDAVBackend()
	adapter := NewFileSystem(drive.New(backend))
	file, err := adapter.OpenFile(context.Background(), "/created.txt", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("created")); err != nil {
		t.Fatal(err)
	}
	if info, err := file.Stat(); err != nil || info.Name() != "created.txt" || info.Size() != 7 {
		t.Fatalf("Stat() = %v, %v", info, err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if got := backend.files["created.txt"]; got == nil || got.data != "created" {
		t.Fatalf("uploaded file = %#v", got)
	}
}

func TestReaddirMaintainsCursor(t *testing.T) {
	backend := newDAVBackend()
	backend.files["dir"] = &davFile{id: "dir", parent: "root", name: "dir", dir: true}
	backend.files["dir/a"] = &davFile{id: "a", parent: "dir", name: "a"}
	backend.files["dir/b"] = &davFile{id: "b", parent: "dir", name: "b"}
	adapter := NewFileSystem(drive.New(backend))
	dir, err := adapter.OpenFile(context.Background(), "/dir", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	first, err := dir.Readdir(1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first Readdir = %v, %v", first, err)
	}
	second, err := dir.Readdir(1)
	if err != nil || len(second) != 1 || second[0].Name() == first[0].Name() {
		t.Fatalf("second Readdir = %v, %v", second, err)
	}
	if entries, err := dir.Readdir(1); !errors.Is(err, io.EOF) || len(entries) != 0 {
		t.Fatalf("final Readdir = %v, %v", entries, err)
	}
}

func TestHandlerEnforcesWriteLocks(t *testing.T) {
	handler := NewHandler("", drive.New(newDAVBackend()))
	lockBody := `<D:lockinfo xmlns:D="DAV:"><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype><D:owner>test</D:owner></D:lockinfo>`
	lock := httptest.NewRequest("LOCK", "/source.txt", strings.NewReader(lockBody))
	lock.Header.Set("Content-Type", "application/xml")
	lockResult := httptest.NewRecorder()
	handler.ServeHTTP(lockResult, lock)
	if lockResult.Code != http.StatusOK {
		t.Fatalf("LOCK status = %d, body = %s", lockResult.Code, lockResult.Body.String())
	}
	put := httptest.NewRequest(http.MethodPut, "/source.txt", strings.NewReader("replacement"))
	putResult := httptest.NewRecorder()
	handler.ServeHTTP(putResult, put)
	if putResult.Code != http.StatusLocked {
		t.Fatalf("locked PUT status = %d, body = %s", putResult.Code, putResult.Body.String())
	}
}
