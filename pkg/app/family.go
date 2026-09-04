package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/gowsp/cloud189/internal/invoker"
	pkg "github.com/gowsp/cloud189/pkg/drive"
)

type FamilyInfo struct {
	ID         json.Number `json:"familyId"`
	RemarkName string      `json:"remarkName"`
	Count      int         `json:"count"`
	UserRole   int         `json:"userRole"`
	Type       int         `json:"type"`
	UseFlag    int         `json:"useFlag"`
	CreateTime string      `json:"createTime"`
	ExpireTime string      `json:"expireTime"`
}

func (f FamilyInfo) FamilyID() string { return f.ID.String() }

func (a *Client) ensureFamilySession(ctx context.Context) error {
	if a.conf.Session != nil && a.conf.Session.FamilyKey != "" && a.conf.Session.FamilySecret != "" {
		return nil
	}
	if err := a.refreshContext(ctx); err != nil {
		return err
	}
	if a.conf.Session == nil || a.conf.Session.FamilyKey == "" || a.conf.Session.FamilySecret == "" {
		return errors.New("登录会话不包含家庭云凭据，请重新登录")
	}
	return nil
}

func (a *Client) Families(ctx context.Context) ([]FamilyInfo, error) {
	if err := a.ensureFamilySession(ctx); err != nil {
		return nil, err
	}
	var response struct {
		Families []FamilyInfo `json:"familyInfoResp"`
	}
	if err := a.invoker.GetScopedContext(ctx, invoker.FamilyScope, "/family/manage/getFamilyList.action", nil, &response); err != nil {
		return nil, err
	}
	return response.Families, nil
}

type familyAPI struct {
	base     *Client
	selector string
	root     pkg.Entry

	mu           sync.RWMutex
	familyID     string
	rootFolderID string
	maxFileSize  int64
}

func (f *familyAPI) ensure(ctx context.Context) error {
	f.mu.RLock()
	ready := f.familyID != ""
	f.mu.RUnlock()
	if ready {
		return nil
	}
	if f.selector == "" {
		return errors.New("家庭 ID 不能为空")
	}
	families, err := f.base.Families(ctx)
	if err != nil {
		return err
	}
	for _, family := range families {
		if family.FamilyID() == f.selector {
			f.mu.Lock()
			f.familyID = family.FamilyID()
			f.mu.Unlock()
			return nil
		}
	}
	return fmt.Errorf("未找到家庭 %q", f.selector)
}

func (f *familyAPI) id() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.familyID
}

func (f *familyAPI) isRoot(target pkg.Entry) bool {
	return target != nil && target.ID() == f.root.ID()
}

func (f *familyAPI) rememberRootID(id string) {
	if id == "" || id == "-16" || id == f.root.ID() {
		return
	}
	f.mu.Lock()
	if f.rootFolderID == "" {
		f.rootFolderID = id
	}
	f.mu.Unlock()
}

func (f *familyAPI) rootID() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.rootFolderID
}

func (f *familyAPI) FamilyID(ctx context.Context) (string, error) {
	if err := f.ensure(ctx); err != nil {
		return "", err
	}
	return f.id(), nil
}
