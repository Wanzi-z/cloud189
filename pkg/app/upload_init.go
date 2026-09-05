package app

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	pkg "github.com/gowsp/cloud189/internal/app/upload"
	"github.com/gowsp/cloud189/internal/invoker"
	"github.com/gowsp/cloud189/internal/util"
)

const defaultUploadURL = "https://upload.cloud.189.cn"

type uploadProfile struct {
	scope        invoker.Scope
	prefix       string
	initMethod   string
	commitMethod string
}

var personalUpload = uploadProfile{
	scope: invoker.PersonalScope, prefix: "/person",
	initMethod: http.MethodPost, commitMethod: http.MethodPost,
}

var familyUpload = uploadProfile{
	scope: invoker.FamilyScope, prefix: "/family",
	initMethod: http.MethodPost, commitMethod: http.MethodPost,
}

type uploader struct {
	api       *Client
	family    *familyAPI
	profile   uploadProfile
	uploadURL string
	ctx       context.Context
}

func newUploader(api *Client, profile uploadProfile) *uploader {
	return &uploader{api: api, profile: profile, uploadURL: defaultUploadURL, ctx: context.Background()}
}

func (client *Client) uploader(ctx context.Context) pkg.Writer {
	uploader := newUploader(client, personalUpload)
	uploader.ctx = ctx
	return uploader
}

func (f *familyAPI) uploader(ctx context.Context) pkg.Writer {
	uploader := newUploader(f.base, familyUpload)
	uploader.family = f
	uploader.ctx = ctx
	return uploader
}

func (u *uploader) Write(source pkg.Source) error {
	if u == nil || u.api == nil || u.api.conf == nil || u.api.invoker == nil {
		return errors.New("upload client is not initialized")
	}
	if source == nil {
		return errors.New("upload source is nil")
	}
	if u.profile.scope == invoker.PersonalScope {
		if err := u.api.invoker.GetContext(u.ctx, "/keepUserSession.action", nil, new(any)); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
	}
	if err := u.validate(source); err != nil {
		return err
	}
	session, err := u.initUpload(source)
	if err != nil {
		return err
	}
	if session.IsExists() {
		return u.commitUpload(source, session.UploadFileId, false)
	}
	if instant, ok := source.(pkg.InstantSource); ok && instant.InstantOnly() {
		return errors.New("服务端未命中秒传，无法在没有本地文件的情况下上传")
	}
	if err := u.uploadParts(source, session.UploadFileId); err != nil {
		return err
	}
	return u.commitUpload(source, session.UploadFileId, true)
}

func (u *uploader) validate(source pkg.Source) error {
	if u.family == nil {
		return nil
	}
	if err := u.family.ensure(u.ctx); err != nil {
		return err
	}
	u.family.mu.RLock()
	maxFileSize := u.family.maxFileSize
	u.family.mu.RUnlock()
	if maxFileSize == 0 {
		if _, err := u.family.Space(u.ctx); err != nil {
			return err
		}
		u.family.mu.RLock()
		maxFileSize = u.family.maxFileSize
		u.family.mu.RUnlock()
	}
	if maxFileSize > 0 && source.Size() > maxFileSize {
		return fmt.Errorf("文件大小 %d 超过家庭云单文件上限 %d", source.Size(), maxFileSize)
	}
	return nil
}

func (u *uploader) familyID() string {
	if u.family == nil {
		return ""
	}
	return u.family.id()
}

func (u *uploader) endpoint(name string) string {
	return u.profile.prefix + "/" + name
}

func (u *uploader) encrypt(params url.Values) (string, error) {
	session := u.api.conf.Session
	if session == nil {
		return "", errors.New("请先登录")
	}
	secret := session.Secret
	if u.profile.scope == invoker.FamilyScope {
		secret = session.FamilySecret
	}
	if len(secret) < 16 {
		return "", errors.New("登录会话无效，请重新登录")
	}
	data := util.AesEncrypt([]byte(util.EncodeParam(params)), []byte(secret[:16]))
	return hex.EncodeToString(data), nil
}

func (u *uploader) request(method, endpoint string, params url.Values, result any) error {
	build := func() (*http.Request, error) {
		encrypted, err := u.encrypt(params)
		if err != nil {
			return nil, err
		}
		form := url.Values{"params": {encrypted}}
		requestURL := u.uploadURL + endpoint
		var body io.Reader
		if method == http.MethodGet {
			requestURL += "?" + form.Encode()
		} else {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequestWithContext(u.ctx, method, requestURL, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("decodefields", "familyId,parentFolderId,fileName,fileMd5,fileSize,sliceMd5,sliceSize,albumId,extend,lazyCheck,isLog")
		req.Header.Set("accept", "application/json;charset=UTF-8")
		req.Header.Set("cache-control", "no-cache")
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		req = invoker.WithScope(req, u.profile.scope)
		req = invoker.WithSignParams(req, encrypted)
		return req, nil
	}
	if err := u.api.invoker.DoBuilt(build, result, 5); err != nil {
		return err
	}
	if response, ok := result.(interface{ GetCode() string }); ok {
		code := response.GetCode()
		if code != "" && code != "SUCCESS" {
			return fmt.Errorf("upload error: %s", code)
		}
	}
	return nil
}

type uploadInfo struct {
	UploadType     int    `json:"uploadType,omitempty"`
	UploadHost     string `json:"uploadHost,omitempty"`
	UploadFileId   string `json:"uploadFileId,omitempty"`
	FileDataExists int    `json:"fileDataExists,omitempty"`
}

func (i *uploadInfo) IsExists() bool { return i.FileDataExists == 1 }

type initResp struct {
	Code string     `json:"code,omitempty"`
	Data uploadInfo `json:"data,omitempty"`
}

func (r *initResp) GetCode() string { return r.Code }

func (u *uploader) initUpload(source pkg.Source) (*uploadInfo, error) {
	params := url.Values{
		"fileName":  {source.Name()},
		"fileSize":  {strconv.FormatInt(source.Size(), 10)},
		"sliceSize": {strconv.Itoa(pkg.SliceSize)},
		"extend":    {`{"opScene":"1","relativepath":"","rootfolderid":""}`},
	}
	if u.profile.scope != invoker.FamilyScope || !strings.HasPrefix(source.ParentID(), "family:") {
		params.Set("parentFolderId", source.ParentID())
	}
	if familyID := u.familyID(); familyID != "" {
		params.Set("familyId", familyID)
	}
	if source.LazyCheck() {
		params.Set("lazyCheck", "1")
	} else {
		params.Set("fileMd5", source.MD5())
		params.Set("sliceMd5", source.SliceMD5())
		if sourceWithError, ok := source.(pkg.ErrorSource); ok {
			if err := sourceWithError.Err(); err != nil {
				return nil, fmt.Errorf("read upload source: %w", err)
			}
		}
	}
	var response initResp
	if err := u.request(u.profile.initMethod, u.endpoint("initMultiUpload"), params, &response); err != nil {
		return nil, err
	}
	if response.Data.UploadFileId == "" {
		return nil, errors.New("error get upload fileid")
	}
	return &response.Data, nil
}

type uploadTarget struct {
	RequestURL    string `json:"requestURL,omitempty"`
	RequestHeader string `json:"requestHeader,omitempty"`
}

type uploadURLResp struct {
	Code string                  `json:"code,omitempty"`
	Data map[string]uploadTarget `json:"uploadUrls,omitempty"`
}

func (r *uploadURLResp) GetCode() string { return r.Code }

func (u *uploader) uploadParts(source pkg.Source, uploadID string) error {
	if os.Getenv("EXE_MODE") == "1" {
		log.Println("start upload", source.Name())
	}
	for index := 0; index < source.SliceCount(); index++ {
		part := source.Part(int64(index))
		if part == nil {
			return fmt.Errorf("upload part %d is nil", index+1)
		}
		if sourceWithError, ok := source.(pkg.ErrorSource); ok {
			if err := sourceWithError.Err(); err != nil {
				return fmt.Errorf("read upload source: %w", err)
			}
		}
		target, err := u.getUploadTarget(uploadID, fmt.Sprintf("%d-%s", index+1, part.Name()))
		if err != nil {
			return err
		}
		if err := u.putPart(part, target); err != nil {
			return err
		}
	}
	if os.Getenv("EXE_MODE") == "1" {
		log.Println("upload", source.Name(), "completed")
	}
	return nil
}

func (u *uploader) getUploadTarget(uploadID, partInfo string) (uploadTarget, error) {
	params := url.Values{"partInfo": {partInfo}, "uploadFileId": {uploadID}}
	var response uploadURLResp
	if err := u.request(http.MethodGet, u.endpoint("getMultiUploadUrls"), params, &response); err != nil {
		return uploadTarget{}, err
	}
	partNumber := strings.SplitN(partInfo, "-", 2)[0]
	target, ok := response.Data["partNumber_"+partNumber]
	if !ok || target.RequestURL == "" {
		return uploadTarget{}, fmt.Errorf("服务端未返回分片 %s 的上传地址", partNumber)
	}
	return target, nil
}

func (u *uploader) putPart(part pkg.Part, target uploadTarget) error {
	req, err := http.NewRequestWithContext(u.ctx, http.MethodPut, target.RequestURL, part.Data())
	if err != nil {
		return err
	}
	for _, header := range strings.Split(target.RequestHeader, "&") {
		key, value, ok := strings.Cut(header, "=")
		if !ok || key == "" {
			return fmt.Errorf("invalid upload header %q", header)
		}
		req.Header.Set(key, value)
	}
	resp, err := u.api.invoker.Send(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return fmt.Errorf("upload part %d returned %s: %s", part.Number()+1, resp.Status, strings.TrimSpace(string(message)))
	}
	return nil
}

type uploadResult struct {
	Code string `json:"code,omitempty"`
	File struct {
		ID         string `json:"userFileId,omitempty"`
		FileSize   int64  `json:"fileSize,omitempty"`
		FileName   string `json:"fileName,omitempty"`
		FileMD5    string `json:"fileMd5,omitempty"`
		CreateDate string `json:"createDate,omitempty"`
	} `json:"file,omitempty"`
}

func (r *uploadResult) GetCode() string { return r.Code }

func (u *uploader) commitUpload(source pkg.Source, uploadID string, uploaded bool) error {
	params := url.Values{"uploadFileId": {uploadID}}
	if uploaded {
		params.Set("fileMd5", source.MD5())
		params.Set("sliceMd5", source.SliceMD5())
		params.Set("lazyCheck", "1")
	}
	if source.Overwrite() {
		params.Set("opertype", "3")
	}
	var response uploadResult
	return u.request(u.profile.commitMethod, u.endpoint("commitMultiUploadFile"), params, &response)
}
