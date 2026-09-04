package drive

import (
	"context"
	"io"
	"io/fs"
	"net/url"
)

type Entry interface {
	fs.FileInfo
	fs.DirEntry
	ID() string
	ParentID() string
}

type FileType uint8

const (
	All FileType = iota
	RegularFile
	Directory
)

type Space struct {
	Available uint64 `json:"available,omitempty"`
	Capacity  uint64 `json:"capacity,omitempty"`
}

type Usage struct {
	Files       uint64 `json:"files,omitempty"`
	Directories uint64 `json:"directories,omitempty"`
	Bytes       uint64 `json:"bytes,omitempty"`
}

type PutOptions struct {
	Overwrite bool
}

type Digest struct {
	MD5      string
	SliceMD5 string
}

type Backend interface {
	Root() Entry
	Stat(context.Context, string) (Entry, error)
	List(context.Context, Entry, FileType) ([]Entry, error)
	Mkdir(context.Context, Entry, string) (Entry, error)
	Rename(context.Context, Entry, string) error
	Move(context.Context, Entry, ...Entry) error
	Copy(context.Context, Entry, ...Entry) error
	Remove(context.Context, ...Entry) error
	Space(context.Context) (Space, error)
	Usage(context.Context, Entry) (Usage, error)
	Open(context.Context, Entry, int64) (io.ReadCloser, error)
	DownloadURL(context.Context, Entry) (*url.URL, error)
	Put(context.Context, Entry, string, io.Reader, int64, PutOptions) error
	PutDigest(context.Context, Entry, string, int64, Digest, PutOptions) error
}
