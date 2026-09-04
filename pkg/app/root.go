package app

import (
	"io/fs"
	"time"

	"github.com/gowsp/cloud189/pkg/drive"
)

var personalRoot drive.Entry = &systemFolder{id: "-11", name: "全部文件"}

func newFamilyRoot(familyID string) drive.Entry {
	return &systemFolder{id: "family:" + familyID, parentID: "-16", name: "家庭云"}
}

var systemFolders = map[string]struct{}{
	"同步盘": {}, "私密空间": {}, "我的图片": {}, "我的视频": {},
	"我的音乐": {}, "我的文档": {}, "我的应用": {},
}

func isSystemFolder(parentID, name string) bool {
	_, ok := systemFolders[name]
	return parentID == personalRoot.ID() && ok
}

type systemFolder struct{ id, parentID, name string }

func (f *systemFolder) ID() string                 { return f.id }
func (f *systemFolder) ParentID() string           { return f.parentID }
func (f *systemFolder) Name() string               { return f.name }
func (*systemFolder) Size() int64                  { return 0 }
func (*systemFolder) Mode() fs.FileMode            { return fs.ModeDir | 0555 }
func (*systemFolder) ModTime() time.Time           { return time.Time{} }
func (*systemFolder) IsDir() bool                  { return true }
func (*systemFolder) Sys() any                     { return nil }
func (*systemFolder) Type() fs.FileMode            { return fs.ModeDir }
func (f *systemFolder) Info() (fs.FileInfo, error) { return f, nil }
