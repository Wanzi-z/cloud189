package upload

import "io"

type Source interface {
	io.Closer
	ParentID() string
	Name() string
	Size() int64
	SliceCount() int
	MD5() string
	SliceMD5() string
	Overwrite() bool
	Part(int64) Part
	LazyCheck() bool
}

type Part interface {
	Number() int
	Name() string
	Data() io.Reader
}

type Writer interface{ Write(Source) error }

type InstantSource interface {
	Source
	InstantOnly() bool
}

type ErrorSource interface {
	Source
	Err() error
}
