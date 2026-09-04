package upload

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"os"
	"strings"
)

const SliceSize = 10 << 20

type fileSource struct {
	parentID, name string
	file           *os.File
	info           os.FileInfo
	partNames      []string
	fileMD5        string
	sliceMD5       string
	overwrite      bool
	err            error
}

func NewFile(parentID, localPath, name string, overwrite bool) (Source, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("upload source must be a regular file")
	}
	count := int(math.Ceil(float64(info.Size()) / float64(SliceSize)))
	return &fileSource{parentID: parentID, name: name, file: file, info: info, partNames: make([]string, count), overwrite: overwrite}, nil
}

func (s *fileSource) Close() error     { return s.file.Close() }
func (s *fileSource) ParentID() string { return s.parentID }
func (s *fileSource) Name() string {
	if s.name != "" {
		return s.name
	}
	return s.info.Name()
}
func (s *fileSource) Size() int64     { return s.info.Size() }
func (s *fileSource) SliceCount() int { return len(s.partNames) }
func (s *fileSource) Overwrite() bool { return s.overwrite }
func (s *fileSource) LazyCheck() bool { return false }
func (s *fileSource) Err() error      { return s.err }
func (s *fileSource) MD5() string {
	if s.fileMD5 == "" {
		_ = s.hash()
	}
	return s.fileMD5
}
func (s *fileSource) SliceMD5() string {
	if s.sliceMD5 == "" {
		_ = s.hash()
	}
	return s.sliceMD5
}
func (s *fileSource) Part(number int64) Part {
	return &sourcePart{number: int(number), name: s.partNames[number], data: io.NewSectionReader(s.file, number*SliceSize, SliceSize)}
}

func (s *fileSource) hash() error {
	whole, partHash := md5.New(), md5.New()
	partDigests := make([]string, s.SliceCount())
	buffer := make([]byte, 32<<10)
	for part := 0; part < s.SliceCount(); part++ {
		reader := io.NewSectionReader(s.file, int64(part*SliceSize), SliceSize)
		if _, err := io.CopyBuffer(io.MultiWriter(whole, partHash), reader, buffer); err != nil {
			s.err = errors.Join(s.err, err)
			return err
		}
		digest := partHash.Sum(nil)
		partDigests[part] = strings.ToUpper(hex.EncodeToString(digest))
		s.partNames[part] = base64.StdEncoding.EncodeToString(digest)
		partHash.Reset()
	}
	s.fileMD5 = strings.ToUpper(hex.EncodeToString(whole.Sum(nil)))
	if len(partDigests) <= 1 {
		s.sliceMD5 = s.fileMD5
		return nil
	}
	_, _ = partHash.Write([]byte(strings.Join(partDigests, "\n")))
	s.sliceMD5 = strings.ToUpper(hex.EncodeToString(partHash.Sum(nil)))
	return nil
}

type sourcePart struct {
	number int
	name   string
	data   io.Reader
}

func (p *sourcePart) Number() int     { return p.number }
func (p *sourcePart) Name() string    { return p.name }
func (p *sourcePart) Data() io.Reader { return p.data }

type digestSource struct {
	parentID, name string
	size           int64
	md5, sliceMD5  string
	overwrite      bool
}

func NewDigest(parentID, name string, size int64, md5sum, sliceMD5 string, overwrite bool) (Source, error) {
	md5sum, sliceMD5 = strings.ToUpper(md5sum), strings.ToUpper(sliceMD5)
	if len(md5sum) != 32 {
		return nil, errors.New("MD5 must contain 32 hexadecimal characters")
	}
	if sliceMD5 == "" {
		if size > SliceSize {
			return nil, errors.New("slice MD5 is required for files larger than 10 MiB")
		}
		sliceMD5 = md5sum
	}
	if len(sliceMD5) != 32 {
		return nil, errors.New("slice MD5 must contain 32 hexadecimal characters")
	}
	return &digestSource{parentID: parentID, name: name, size: size, md5: md5sum, sliceMD5: sliceMD5, overwrite: overwrite}, nil
}
func (s *digestSource) Close() error      { return nil }
func (s *digestSource) ParentID() string  { return s.parentID }
func (s *digestSource) Name() string      { return s.name }
func (s *digestSource) Size() int64       { return s.size }
func (s *digestSource) SliceCount() int   { return 0 }
func (s *digestSource) MD5() string       { return s.md5 }
func (s *digestSource) SliceMD5() string  { return s.sliceMD5 }
func (s *digestSource) Overwrite() bool   { return s.overwrite }
func (s *digestSource) Part(int64) Part   { return nil }
func (s *digestSource) LazyCheck() bool   { return false }
func (s *digestSource) InstantOnly() bool { return true }
