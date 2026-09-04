package app

import (
	"encoding/json"
	"io/fs"
	"strings"
	"time"
)

type cloudTime time.Time

func (j *cloudTime) UnmarshalJSON(b []byte) error {
	json := string(b)
	s := strings.Trim(json, "\"")
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return err
	}
	*j = cloudTime(t)
	return nil
}

type folder struct {
	FileID       json.Number `json:"id"`
	ParentFileID json.Number `json:"parentId"`
	FileCata     int         `json:"fileCata"`
	FileCount    int         `json:"fileCount"`
	FileListSize int         `json:"fileListSize"`
	LastOpTime   cloudTime   `json:"lastOpTime"`
	CreateDate   cloudTime   `json:"createDate"`
	DirName      string      `json:"name"`
	Rev          string      `json:"rev"`
	StarLabel    int         `json:"starLabel"`
}

func (f *folder) Info() (fs.FileInfo, error) { return f, nil }

func (f *folder) ID() string         { return f.FileID.String() }
func (f *folder) ParentID() string   { return f.ParentFileID.String() }
func (f *folder) Name() string       { return f.DirName }
func (f *folder) Size() int64        { return 0 }
func (f *folder) Type() fs.FileMode  { return fs.ModeDir }
func (f *folder) Mode() fs.FileMode  { return fs.ModeDir }
func (f *folder) ModTime() time.Time { return time.Time(f.LastOpTime) }
func (f *folder) IsDir() bool        { return true }
func (f *folder) Sys() any           { return nil }

type fileInfo struct {
	ParentFileID json.Number `json:"parentId"`
	FileID       json.Number `json:"id"`

	Md5         string    `json:"md5"`
	MediaType   int       `json:"mediaType"`
	FileCata    int       `json:"fileCata"`
	FileName    string    `json:"name"`
	FileSize    int64     `json:"size"`
	Orientation int       `json:"orientation"`
	Rev         string    `json:"rev"`
	StarLabel   int       `json:"starLabel"`
	LastOpTime  cloudTime `json:"lastOpTime"`
	CreateDate  cloudTime `json:"createDate"`
}

func (f *fileInfo) Info() (fs.FileInfo, error) { return f, nil }

func (f *fileInfo) ID() string         { return f.FileID.String() }
func (f *fileInfo) ParentID() string   { return f.ParentFileID.String() }
func (f *fileInfo) Name() string       { return f.FileName }
func (f *fileInfo) Size() int64        { return f.FileSize }
func (f *fileInfo) Mode() fs.FileMode  { return 0444 }
func (f *fileInfo) Type() fs.FileMode  { return 0 }
func (f *fileInfo) ModTime() time.Time { return time.Time(f.LastOpTime) }
func (f *fileInfo) IsDir() bool        { return false }
func (f *fileInfo) Sys() any           { return nil }
