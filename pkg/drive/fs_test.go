package drive

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

type memoryFile struct {
	id, parent, name, data string
	dir                    bool
}

func (f *memoryFile) ID() string       { return f.id }
func (f *memoryFile) ParentID() string { return f.parent }
func (f *memoryFile) Name() string     { return f.name }
func (f *memoryFile) Size() int64      { return int64(len(f.data)) }
func (f *memoryFile) Mode() fs.FileMode {
	if f.dir {
		return fs.ModeDir | 0555
	}
	return 0444
}
func (f *memoryFile) ModTime() time.Time         { return time.Time{} }
func (f *memoryFile) IsDir() bool                { return f.dir }
func (f *memoryFile) Sys() any                   { return nil }
func (f *memoryFile) Type() fs.FileMode          { return f.Mode().Type() }
func (f *memoryFile) Info() (fs.FileInfo, error) { return f, nil }

type memoryBackend struct {
	files       map[string]*memoryFile
	removeCalls int
	removed     int
}

func newMemoryBackend() *memoryBackend {
	return &memoryBackend{files: map[string]*memoryFile{
		"hello.txt":      {id: "hello", parent: "root", name: "hello.txt", data: "hello"},
		"dir":            {id: "dir", parent: "root", name: "dir", dir: true},
		"dir/nested.txt": {id: "nested", parent: "dir", name: "nested.txt", data: "nested"},
	}}
}

func (*memoryBackend) Root() Entry                                 { return &memoryFile{id: "root", name: ".", dir: true} }
func (*memoryBackend) Space(context.Context) (Space, error)        { return Space{}, nil }
func (*memoryBackend) Usage(context.Context, Entry) (Usage, error) { return Usage{}, nil }
func (b *memoryBackend) Mkdir(_ context.Context, parent Entry, name string) (Entry, error) {
	entry := &memoryFile{id: name, parent: parent.ID(), name: name, dir: true}
	b.files[name] = entry
	return entry, nil
}
func (b *memoryBackend) Rename(_ context.Context, entry Entry, name string) error {
	for key, candidate := range b.files {
		if candidate == entry {
			delete(b.files, key)
			candidate.name = name
			b.files[name] = candidate
			return nil
		}
	}
	return fs.ErrNotExist
}
func (*memoryBackend) Move(context.Context, Entry, ...Entry) error { return nil }
func (*memoryBackend) Copy(context.Context, Entry, ...Entry) error { return fs.ErrPermission }
func (*memoryBackend) MoveWithOptions(ctx context.Context, t Entry, _ ConflictPolicy, s ...Entry) error {
	return nil
}
func (*memoryBackend) CopyWithOptions(ctx context.Context, t Entry, _ ConflictPolicy, s ...Entry) error {
	return fs.ErrPermission
}
func (b *memoryBackend) Remove(_ context.Context, entries ...Entry) error {
	b.removeCalls++
	b.removed = len(entries)
	for key, candidate := range b.files {
		for _, entry := range entries {
			if candidate == entry {
				delete(b.files, key)
			}
		}
	}
	return nil
}

func TestBatchRemoveUsesSingleBackendCall(t *testing.T) {
	backend := newMemoryBackend()
	client := New(backend)
	if err := client.Remove(context.Background(), "hello.txt", "dir/nested.txt"); err != nil {
		t.Fatal(err)
	}
	if backend.removeCalls != 1 || backend.removed != 2 {
		t.Fatalf("remove calls=%d entries=%d", backend.removeCalls, backend.removed)
	}
}
func (b *memoryBackend) Put(_ context.Context, parent Entry, name string, reader io.Reader, size int64, options PutOptions) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if int64(len(data)) != size {
		return fs.ErrInvalid
	}
	b.files[name] = &memoryFile{id: name, parent: parent.ID(), name: name, data: string(data)}
	return nil
}
func (b *memoryBackend) PutDigest(_ context.Context, parent Entry, name string, size int64, digest Digest, options PutOptions) error {
	b.files[name] = &memoryFile{id: digest.MD5, parent: parent.ID(), name: name, data: strings.Repeat("x", int(size))}
	return nil
}
func (*memoryBackend) DownloadURL(context.Context, Entry) (*url.URL, error) {
	return nil, fs.ErrPermission
}
func (*memoryBackend) Open(_ context.Context, entry Entry, _ int64) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(entry.(*memoryFile).data)), nil
}

func (b *memoryBackend) Stat(_ context.Context, name string) (Entry, error) {
	if item := b.files[name]; item != nil {
		return item, nil
	}
	return nil, fs.ErrNotExist
}

func (b *memoryBackend) List(_ context.Context, parent Entry, _ FileType) ([]Entry, error) {
	result := []Entry{}
	for _, item := range b.files {
		if item.parent == parent.ID() {
			result = append(result, item)
		}
	}
	return result, nil
}

func TestFSConformsToIOFS(t *testing.T) {
	client := New(newMemoryBackend())
	if err := fstest.TestFS(client, "hello.txt", "dir/nested.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestFSRejectsNativeAbsolutePaths(t *testing.T) {
	client := New(newMemoryBackend())
	for _, name := range []string{"/", "/dir", "dir/../hello.txt", ""} {
		if _, err := client.StatContext(context.Background(), name); err == nil || !strings.Contains(err.Error(), "invalid") {
			t.Fatalf("StatContext(%q) error = %v", name, err)
		}
	}
	if info, err := client.Stat("."); err != nil || info.Name() != "." {
		t.Fatalf("Stat(.) = %v, %v", info, err)
	}
}

func TestFSErrorsIncludeOperationAndPath(t *testing.T) {
	client := New(newMemoryBackend())
	checks := []struct {
		op   string
		call func() error
	}{
		{op: "open", call: func() error { _, err := client.Open("missing"); return err }},
		{op: "stat", call: func() error { _, err := client.Stat("missing"); return err }},
		{op: "readdir", call: func() error { _, err := client.ReadDir("hello.txt"); return err }},
	}
	for _, check := range checks {
		err := check.call()
		var pathErr *fs.PathError
		if !errors.As(err, &pathErr) || pathErr.Op != check.op {
			t.Errorf("error = %v, want PathError operation %q", err, check.op)
		}
	}
}

func TestPutDownloadAndDigest(t *testing.T) {
	client := New(newMemoryBackend())
	entry, err := client.Put(context.Background(), "new.txt", strings.NewReader("new"), 3, PutOptions{})
	if err != nil || entry.ID() != "new.txt" {
		t.Fatalf("Put() = %v, %v", entry, err)
	}
	var downloaded strings.Builder
	if _, err := client.Download(context.Background(), "new.txt", &downloaded); err != nil || downloaded.String() != "new" {
		t.Fatalf("Download() = %q, %v", downloaded.String(), err)
	}
	entry, err = client.PutDigest(context.Background(), "digest.txt", 2, Digest{MD5: "D41D8CD98F00B204E9800998ECF8427E"}, PutOptions{})
	if err != nil || entry.Size() != 2 {
		t.Fatalf("PutDigest() = %v, %v", entry, err)
	}
	if _, err := client.Put(context.Background(), "new.txt", strings.NewReader("different"), 9, PutOptions{}); err == nil {
		t.Fatal("Put accepted conflicting file without overwrite")
	}
}

func TestNativeMutations(t *testing.T) {
	client := New(newMemoryBackend())
	if err := client.Mkdir(context.Background(), "newdir", 0700); err != nil {
		t.Fatal(err)
	}
	if err := client.Rename(context.Background(), "hello.txt", "renamed.txt"); err != nil {
		t.Fatal(err)
	}
	entry, err := client.StatContext(context.Background(), "renamed.txt")
	if err != nil || entry.ID() != "hello" {
		t.Fatalf("renamed entry = %v, %v", entry, err)
	}
	if err := client.RemoveAll(context.Background(), "renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.StatContext(context.Background(), "renamed.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("removed stat error = %v", err)
	}
}

func TestRenameDoesNotDeleteExistingDestination(t *testing.T) {
	backend := newMemoryBackend()
	client := New(backend)
	err := client.Rename(context.Background(), "hello.txt", "dir/nested.txt")
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("Rename error = %v, want ErrExist", err)
	}
	if backend.files["hello.txt"] == nil || backend.files["dir/nested.txt"] == nil {
		t.Fatalf("rename collision changed files: %#v", backend.files)
	}
}

func TestCopyAndMoveRejectMissingOperands(t *testing.T) {
	client := New(newMemoryBackend())
	if err := client.Copy(context.Background(), "dir", "hello.txt", "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Copy error = %v, want ErrNotExist", err)
	}
	if err := client.move(context.Background(), "dir", "hello.txt", "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("move error = %v, want ErrNotExist", err)
	}
}

var _ fs.FS = (*FS)(nil)
var _ fs.StatFS = (*FS)(nil)
var _ fs.ReadDirFS = (*FS)(nil)
