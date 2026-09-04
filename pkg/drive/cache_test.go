package drive

import (
	"io/fs"
	"testing"
	"time"
)

type cacheTestFile struct {
	id     string
	parent string
	name   string
}

func (f *cacheTestFile) ID() string                 { return f.id }
func (f *cacheTestFile) ParentID() string           { return f.parent }
func (f *cacheTestFile) Name() string               { return f.name }
func (f *cacheTestFile) Size() int64                { return 0 }
func (f *cacheTestFile) Mode() fs.FileMode          { return 0 }
func (f *cacheTestFile) ModTime() time.Time         { return time.Time{} }
func (f *cacheTestFile) IsDir() bool                { return false }
func (f *cacheTestFile) Sys() any                   { return nil }
func (f *cacheTestFile) Type() fs.FileMode          { return f.Mode().Type() }
func (f *cacheTestFile) Info() (fs.FileInfo, error) { return f, nil }

func TestFileCacheIsIsolatedPerDrive(t *testing.T) {
	firstRoot := &cacheTestFile{id: "family:1", name: "."}
	secondRoot := &cacheTestFile{id: "family:2", name: "."}
	first := newFileCache(firstRoot)
	second := newFileCache(secondRoot)

	firstRootNode := first.load(firstRoot.ID())
	first.alias("real-root", firstRootNode)
	firstRootNode.add(&cacheTestFile{id: "file", parent: "real-root", name: "a.txt"})

	if first.load("real-root") == nil || first.load("file") == nil {
		t.Fatal("first cache did not retain aliases and children")
	}
	if second.load("real-root") != nil || second.load("file") != nil {
		t.Fatal("cache entries leaked between drive instances")
	}
}
