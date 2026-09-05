package app

import (
	"context"
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	pkg "github.com/gowsp/cloud189/internal/app/upload"
	"github.com/gowsp/cloud189/internal/invoker"
	"github.com/gowsp/cloud189/internal/util"
	drivepkg "github.com/gowsp/cloud189/pkg/drive"
)

func decryptUploadParams(t *testing.T, raw, secret string) url.Values {
	t.Helper()
	ciphertext, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher([]byte(secret[:16]))
	if err != nil {
		t.Fatal(err)
	}
	plaintext := make([]byte, len(ciphertext))
	for start := 0; start < len(ciphertext); start += block.BlockSize() {
		block.Decrypt(plaintext[start:start+block.BlockSize()], ciphertext[start:start+block.BlockSize()])
	}
	padding := int(plaintext[len(plaintext)-1])
	if padding < 1 || padding > block.BlockSize() {
		t.Fatalf("invalid padding %d", padding)
	}
	params, err := url.ParseQuery(string(plaintext[:len(plaintext)-padding]))
	if err != nil {
		t.Fatal(err)
	}
	return params
}

type staticUpload struct {
	parentID string
	name     string
	data     string
}

func (u *staticUpload) ParentID() string { return u.parentID }
func (u *staticUpload) Name() string     { return u.name }
func (u *staticUpload) Size() int64      { return int64(len(u.data)) }
func (u *staticUpload) SliceCount() int  { return 1 }
func (u *staticUpload) MD5() string      { return "8D777F385D3DFEC8815D20F7496026DC" }
func (u *staticUpload) SliceMD5() string { return u.MD5() }
func (u *staticUpload) Overwrite() bool  { return false }
func (u *staticUpload) LazyCheck() bool  { return false }
func (u *staticUpload) Close() error     { return nil }
func (u *staticUpload) Part(int64) pkg.Part {
	return &staticUploadPart{data: u.data}
}

type staticUploadPart struct{ data string }

func (p *staticUploadPart) Number() int     { return 0 }
func (p *staticUploadPart) Name() string    { return "jXd/OF09/siBXSd/SWAmbA==" }
func (p *staticUploadPart) Data() io.Reader { return strings.NewReader(p.data) }

func newTestFamilyAPI(t *testing.T, serverURL string) *familyAPI {
	t.Helper()
	t.Setenv("189_MODE", "")
	conf := &invoker.Config{Session: &invoker.Session{
		Key: "personal-key", Secret: "personal-secret-0123456789",
		FamilyKey: "family-key", FamilySecret: "family-secret-0123456789",
	}}
	base := &Client{conf: conf}
	base.invoker = invoker.NewInvoker(serverURL, func(context.Context) error { return nil }, conf)
	base.invoker.SetPrepareE(base.sign)
	selector := "123"
	return &familyAPI{base: base, selector: selector, familyID: selector, root: newFamilyRoot(selector)}
}

func TestClientFamilyValidatesEagerly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/family/manage/getFamilyList.action" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"familyInfoResp":[{"familyId":123,"remarkName":"home"}]}`)
	}))
	defer server.Close()
	family := newTestFamilyAPI(t, server.URL)
	client := family.base
	filesystem, err := client.Family(context.Background(), "123")
	if err != nil || filesystem == nil {
		t.Fatalf("Family() = %v, %v", filesystem, err)
	}
	if _, err := client.Family(context.Background(), "missing"); err == nil {
		t.Fatal("Family accepted an unknown selector")
	}
	if _, err := client.Family(context.Background(), "home"); err == nil {
		t.Fatal("Family accepted a remark name instead of an ID")
	}
}

func newTestFamilyUploader(family *familyAPI, uploadURL string) *uploader {
	family.mu.Lock()
	if family.maxFileSize == 0 {
		family.maxFileSize = 1 << 40
	}
	family.mu.Unlock()
	uploader := newUploader(family.base, familyUpload)
	uploader.family = family
	uploader.uploadURL = uploadURL
	return uploader
}

func TestFamilyListRootOmitsFolderID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("folderId"); got != "" {
			t.Errorf("folderId = %q, want omitted", got)
		}
		if got := r.Header.Get("SessionKey"); got != "family-key" {
			t.Errorf("SessionKey = %q", got)
		}
		_, _ = io.WriteString(w, `{"fileListAO":{"count":1,"fileList":[{"id":2,"parentId":"99","name":"a.txt","size":1}],"folderList":[]}}`)
	}))
	defer server.Close()
	family := newTestFamilyAPI(t, server.URL)
	entries, err := family.List(context.Background(), family.root, drivepkg.All)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ParentID() != "99" || family.rootID() != "99" {
		t.Fatalf("entries=%v parent=%q rootID=%q", len(entries), entries[0].ParentID(), family.rootID())
	}
}

func TestFamilyMkdirRootAndChild(t *testing.T) {
	created := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/family/file/listFiles.action":
			_, _ = io.WriteString(w, `{"fileListAO":{"count":0,"fileList":[],"folderList":[]}}`)
		case "/family/file/createFolder.action":
			created++
			parent := r.URL.Query().Get("parentId")
			if created == 1 && parent != "" {
				t.Errorf("root parentId = %q, want omitted", parent)
			}
			if created == 2 && parent != "10" {
				t.Errorf("child parentId = %q, want 10", parent)
			}
			id, parentID := 10, 99
			if created == 2 {
				id, parentID = 11, 10
			}
			_, _ = fmt.Fprintf(w, `{"id":%d,"parentId":%d,"name":"dir"}`, id, parentID)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	family := newTestFamilyAPI(t, server.URL)
	rootChild, err := family.Mkdir(context.Background(), family.root, "one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = family.Mkdir(context.Background(), rootChild, "two"); err != nil {
		t.Fatal(err)
	}
}

func TestFamilyDriveMkdirUsesFamilyRoot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/family/file/listFiles.action":
			if r.URL.Query().Get("folderId") != "" {
				t.Errorf("root list folderId = %q", r.URL.Query().Get("folderId"))
			}
			_, _ = io.WriteString(w, `{"fileListAO":{"count":0,"fileList":[],"folderList":[]}}`)
		case "/family/file/createFolder.action":
			if r.URL.Query().Get("parentId") != "" {
				t.Errorf("root mkdir parentId = %q", r.URL.Query().Get("parentId"))
			}
			_, _ = io.WriteString(w, `{"id":10,"parentId":99,"name":"new"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	family := newTestFamilyAPI(t, server.URL)
	drive := drivepkg.New(&familyBackend{family: family})
	if err := drive.Mkdir(context.Background(), "new", 0755); err != nil {
		t.Fatal(err)
	}
}

func TestFamilyMoveToRootUsesZeroAndPolls(t *testing.T) {
	created := false
	polled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/batch/createBatchTask.action":
			created = true
			if r.Form.Get("targetFolderId") != "0" || r.Form.Get("type") != "MOVE" {
				t.Errorf("create form = %v", r.Form)
			}
			_, _ = io.WriteString(w, `{"res_code":0,"taskId":"task-1"}`)
		case "/batch/checkBatchTask.action":
			polled = true
			if r.URL.Query().Get("familyId") != "123" || r.Form.Get("taskId") != "task-1" {
				t.Errorf("poll query=%v form=%v", r.URL.Query(), r.Form)
			}
			_, _ = io.WriteString(w, `{"res_code":0,"taskStatus":4,"successedCount":1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	family := newTestFamilyAPI(t, server.URL)
	source := &fileInfo{FileID: json.Number("7"), ParentFileID: json.Number("8"), FileName: "a.txt"}
	if err := family.Move(context.Background(), family.root, source); err != nil {
		t.Fatal(err)
	}
	if !created || !polled {
		t.Fatalf("created=%v polled=%v", created, polled)
	}
}

func TestFamilyDownloadUsesScopedURLAndRange(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/family/file/getFileDownloadUrl.action", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("SessionKey") != "family-key" {
			t.Errorf("SessionKey = %q", r.Header.Get("SessionKey"))
		}
		_, _ = io.WriteString(w, `{"fileDownloadUrl":"`+server.URL+`/blob?token=one&amp;sig=two"}`)
	})
	mux.HandleFunc("/blob", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=5-" {
			t.Errorf("Range = %q", r.Header.Get("Range"))
		}
		if r.URL.Query().Get("sig") != "two" {
			t.Errorf("sig = %q, query=%v", r.URL.Query().Get("sig"), r.URL.Query())
		}
		_, _ = io.WriteString(w, "data")
	})
	family := newTestFamilyAPI(t, server.URL)
	entry := &fileInfo{FileID: json.Number("7"), FileName: "a.txt"}
	response, err := family.Download(context.Background(), entry, 5)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
}

func TestFamilyRootUsageUsesDiscoveredRootID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/family/file/listFiles.action":
			_, _ = io.WriteString(w, `{"fileListAO":{"count":1,"fileList":[{"id":2,"parentId":"99","name":"a.txt","size":10}],"folderList":[]}}`)
		case "/file/createFolderExtInfoTask.action":
			if r.URL.Query().Get("folderId") != "99" {
				t.Errorf("folderId = %q, want 99", r.URL.Query().Get("folderId"))
			}
			_, _ = io.WriteString(w, `{"res_code":0,"taskId":"usage-1"}`)
		case "/file/queryTaskResult.action":
			if r.Header.Get("SessionKey") != "family-key" {
				t.Errorf("SessionKey = %q", r.Header.Get("SessionKey"))
			}
			_, _ = io.WriteString(w, `{"res_code":0,"taskStatus":4,"fileCount":1,"fileSize":10,"folderCount":0}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	family := newTestFamilyAPI(t, server.URL)
	usage, err := family.DirUsage(context.Background(), family.root)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Files != 1 || usage.Bytes != 10 {
		t.Fatalf("files=%d size=%d", usage.Files, usage.Bytes)
	}
}

func TestFamilyUploadPostSignsEncryptedParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		encrypted := r.Form.Get("params")
		if encrypted == "" {
			t.Fatal("missing encrypted params")
		}
		date := r.Header.Get("Date")
		data := "SessionKey=family-key&Operate=POST&RequestURI=/family/initMultiUpload&Date=" + date + "&params=" + encrypted
		if got, want := r.Header.Get("Signature"), util.Sha1(data, "family-secret-0123456789"); got != want {
			t.Errorf("Signature = %q, want %q", got, want)
		}
		_, _ = io.WriteString(w, `{"code":"SUCCESS","data":{"uploadFileId":"id","fileDataExists":1}}`)
	}))
	defer server.Close()
	family := newTestFamilyAPI(t, server.URL)
	upload := newTestFamilyUploader(family, server.URL)
	var response initResp
	if err := upload.request(http.MethodPost, "/family/initMultiUpload", url.Values{"familyId": {"123"}}, &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.UploadFileId != "id" {
		t.Fatalf("upload id = %q", response.Data.UploadFileId)
	}
}

func TestFamilyUploadReencryptsAfterRefresh(t *testing.T) {
	t.Setenv("189_MODE", "")
	conf := &invoker.Config{Session: &invoker.Session{
		Key: "personal-key", Secret: "personal-secret-0123456789",
		FamilyKey: "old-family-key", FamilySecret: "old-family-secret-123456",
	}}
	requests := 0
	var ciphertexts []string
	var sessionKeys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		ciphertexts = append(ciphertexts, r.Form.Get("params"))
		sessionKeys = append(sessionKeys, r.Header.Get("SessionKey"))
		if requests == 1 {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"errorCode":"InvalidSessionKey","errorMsg":"expired"}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":"SUCCESS","data":{"uploadFileId":"id"}}`)
	}))
	defer server.Close()

	base := &Client{conf: conf}
	base.invoker = invoker.NewInvoker(server.URL, func(context.Context) error {
		conf.Session = &invoker.Session{
			Key: "personal-key", Secret: "personal-secret-0123456789",
			FamilyKey: "new-family-key", FamilySecret: "new-family-secret-123456",
		}
		return nil
	}, conf)
	base.invoker.SetPrepareE(base.sign)
	family := &familyAPI{base: base, selector: "123", familyID: "123", root: newFamilyRoot("123")}
	upload := newTestFamilyUploader(family, server.URL)
	var response initResp
	if err := upload.request(http.MethodPost, "/family/initMultiUpload", url.Values{"familyId": {"123"}}, &response); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || sessionKeys[0] != "old-family-key" || sessionKeys[1] != "new-family-key" {
		t.Fatalf("requests=%d sessionKeys=%v", requests, sessionKeys)
	}
	if ciphertexts[0] == ciphertexts[1] {
		t.Fatal("encrypted params were reused after session refresh")
	}
}

func TestFastUploadMissDoesNotCommit(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/family/initMultiUpload" {
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"code":"SUCCESS","data":{"uploadFileId":"id","fileDataExists":0}}`)
	}))
	defer server.Close()
	family := newTestFamilyAPI(t, server.URL)
	uploader := newTestFamilyUploader(family, server.URL)
	fast, err := pkg.NewDigest(family.root.ID(), "empty.txt", 0, "D41D8CD98F00B204E9800998ECF8427E", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := uploader.Write(fast); err == nil || !strings.Contains(err.Error(), "未命中秒传") {
		t.Fatalf("error=%v", err)
	}
	if requests != 1 {
		t.Fatalf("requests=%d, want init only", requests)
	}
}

func TestFastUploadHitCommitsDirectly(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		params := decryptUploadParams(t, r.Form.Get("params"), "family-secret-0123456789")
		switch r.URL.Path {
		case "/family/initMultiUpload":
			if params.Get("familyId") != "123" || params.Get("parentFolderId") != "" || params.Get("fileSize") != "0" {
				t.Errorf("init params=%v", params)
			}
			_, _ = io.WriteString(w, `{"code":"SUCCESS","data":{"uploadFileId":"id","fileDataExists":1}}`)
		case "/family/commitMultiUploadFile":
			if params.Get("uploadFileId") != "id" || params.Get("lazyCheck") != "" {
				t.Errorf("commit params=%v", params)
			}
			_, _ = io.WriteString(w, `{"code":"SUCCESS","file":{"userFileId":"file-id"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	family := newTestFamilyAPI(t, server.URL)
	uploader := newTestFamilyUploader(family, server.URL)
	fast, err := pkg.NewDigest(family.root.ID(), "empty.txt", 0, "D41D8CD98F00B204E9800998ECF8427E", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := uploader.Write(fast); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths, ","); got != "/family/initMultiUpload,/family/commitMultiUploadFile" {
		t.Fatalf("request paths=%s", got)
	}
}

func TestFamilyMultipartUploadProtocol(t *testing.T) {
	var paths []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/family/initMultiUpload":
			if r.Method != http.MethodPost {
				t.Errorf("init method=%s", r.Method)
			}
			_ = r.ParseForm()
			params := decryptUploadParams(t, r.Form.Get("params"), "family-secret-0123456789")
			if params.Get("familyId") != "123" || params.Get("parentFolderId") != "" {
				t.Errorf("init params=%v", params)
			}
			_, _ = io.WriteString(w, `{"code":"SUCCESS","data":{"uploadFileId":"id","fileDataExists":0}}`)
		case "/family/getMultiUploadUrls":
			if r.Method != http.MethodGet {
				t.Errorf("get urls method=%s", r.Method)
			}
			params := decryptUploadParams(t, r.URL.Query().Get("params"), "family-secret-0123456789")
			if params.Get("uploadFileId") != "id" || !strings.HasPrefix(params.Get("partInfo"), "1-") {
				t.Errorf("get urls params=%v", params)
			}
			_, _ = fmt.Fprintf(w, `{"code":"SUCCESS","uploadUrls":{"partNumber_1":{"requestURL":%q,"requestHeader":"Content-Type=application/octet-stream"}}}`, server.URL+"/part")
		case "/part":
			if r.Method != http.MethodPut {
				t.Errorf("part method=%s", r.Method)
			}
			data, _ := io.ReadAll(r.Body)
			if string(data) != "data" {
				t.Errorf("part data=%q", data)
			}
		case "/family/commitMultiUploadFile":
			if r.Method != http.MethodPost {
				t.Errorf("commit method=%s", r.Method)
			}
			_ = r.ParseForm()
			params := decryptUploadParams(t, r.Form.Get("params"), "family-secret-0123456789")
			if params.Get("uploadFileId") != "id" || params.Get("lazyCheck") != "1" ||
				params.Get("fileMd5") != "8D777F385D3DFEC8815D20F7496026DC" ||
				params.Get("sliceMd5") != "8D777F385D3DFEC8815D20F7496026DC" {
				t.Errorf("commit params=%v", params)
			}
			_, _ = io.WriteString(w, `{"code":"SUCCESS","file":{"userFileId":"file-id"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	family := newTestFamilyAPI(t, server.URL)
	uploader := newTestFamilyUploader(family, server.URL)
	if err := uploader.Write(&staticUpload{parentID: family.root.ID(), name: "file.txt", data: "data"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths, ","); got != "/family/initMultiUpload,/family/getMultiUploadUrls,/part,/family/commitMultiUploadFile" {
		t.Fatalf("paths=%s", got)
	}
}

func TestPersonalUploadProfile(t *testing.T) {
	var paths []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/keepUserSession.action" && r.Method != http.MethodPost {
			t.Errorf("method=%s path=%s", r.Method, r.URL.Path)
		}
		if r.URL.Path == "/keepUserSession.action" {
			_, _ = io.WriteString(w, `{}`)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		params := decryptUploadParams(t, r.Form.Get("params"), "personal-secret-0123456789")
		switch r.URL.Path {
		case "/person/initMultiUpload":
			if params.Get("parentFolderId") != "-11" || params.Get("familyId") != "" {
				t.Errorf("init params=%v", params)
			}
			_, _ = io.WriteString(w, `{"code":"SUCCESS","data":{"uploadFileId":"id","fileDataExists":1}}`)
		case "/person/commitMultiUploadFile":
			if params.Get("uploadFileId") != "id" {
				t.Errorf("commit params=%v", params)
			}
			_, _ = io.WriteString(w, `{"code":"SUCCESS","file":{"userFileId":"file-id"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base := newTestFamilyAPI(t, server.URL).base
	uploader := newUploader(base, personalUpload)
	uploader.uploadURL = server.URL
	fast, err := pkg.NewDigest("-11", "empty.txt", 0, "D41D8CD98F00B204E9800998ECF8427E", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := uploader.Write(fast); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths, ","); got != "/keepUserSession.action,/person/initMultiUpload,/person/commitMultiUploadFile" {
		t.Fatalf("paths=%s", got)
	}
}

func TestPersonalUploadKeepaliveUsesContext(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, `{}`)
	}))
	defer func() {
		close(release)
		server.Close()
	}()
	base := newTestFamilyAPI(t, server.URL).base
	ctx, cancel := context.WithCancel(context.Background())
	uploader := newUploader(base, personalUpload)
	uploader.ctx = ctx
	fast, err := pkg.NewDigest("-11", "empty.txt", 0, "D41D8CD98F00B204E9800998ECF8427E", "", false)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- uploader.Write(fast) }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("upload error = %v, want context cancellation", err)
	}
}

func TestZeroUploadReturnsInitializationError(t *testing.T) {
	var uploader uploader
	fast, err := pkg.NewDigest("-11", "empty.txt", 0, "D41D8CD98F00B204E9800998ECF8427E", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := uploader.Write(fast); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("error=%v", err)
	}
}

func TestBatchConflictResolutionProtocol(t *testing.T) {
	for _, policy := range []struct {
		name    string
		policy  ConflictPolicy
		dealWay int
	}{
		{name: "skip", policy: ConflictSkip, dealWay: 1},
		{name: "keep both", policy: ConflictKeepBoth, dealWay: 2},
		{name: "overwrite", policy: ConflictOverwrite, dealWay: 3},
	} {
		t.Run(policy.name, func(t *testing.T) {
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				switch r.URL.Path {
				case "/batch/createBatchTask.action":
					_, _ = io.WriteString(w, `{"res_code":0,"taskId":"conflict-task"}`)
				case "/batch/checkBatchTask.action":
					if len(paths) == 2 {
						_, _ = io.WriteString(w, `{"res_code":0,"taskStatus":2,"subTaskCount":1,"successedCount":0}`)
						return
					}
					_, _ = io.WriteString(w, `{"res_code":0,"taskStatus":4,"subTaskCount":1,"successedCount":1}`)
				case "/batch/getConflictTaskInfo.action":
					_, _ = io.WriteString(w, `{"res_code":0,"taskId":"conflict-task","taskInfos":[{"fileId":"7","fileName":"a.txt","isFolder":0}]}`)
				case "/batch/manageBatchTask.action":
					taskInfos := r.Form.Get("taskInfos")
					want := fmt.Sprintf(`"dealWay":%d`, policy.dealWay)
					if !strings.Contains(taskInfos, want) {
						t.Errorf("manage taskInfos=%s, want %s", taskInfos, want)
					}
					_, _ = io.WriteString(w, `{"res_code":0,"success":true}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := newTestFamilyAPI(t, server.URL).base
			batch := client.personalBatch()
			batch.conflict = policy.policy
			target := &folder{FileID: "10", DirName: "target"}
			source := &fileInfo{FileID: "7", ParentFileID: json.Number("8"), FileName: "a.txt"}
			if err := batch.run(context.Background(), "COPY", targetID(target), source); err != nil {
				t.Fatal(err)
			}
			want := "/batch/createBatchTask.action,/batch/checkBatchTask.action,/batch/getConflictTaskInfo.action,/batch/manageBatchTask.action,/batch/checkBatchTask.action"
			if got := strings.Join(paths, ","); got != want {
				t.Fatalf("paths=%s", got)
			}
		})
	}
}

func TestBatchConflictErrorPolicyKeepsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/batch/createBatchTask.action":
			_, _ = io.WriteString(w, `{"res_code":0,"taskId":"task"}`)
		case "/batch/checkBatchTask.action":
			_, _ = io.WriteString(w, `{"res_code":0,"taskStatus":2,"subTaskCount":1,"successedCount":0}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newTestFamilyAPI(t, server.URL).base
	target := &folder{FileID: "10", DirName: "target"}
	source := &fileInfo{FileID: "7", ParentFileID: json.Number("8"), FileName: "a.txt"}
	if err := client.Copy(context.Background(), target, source); err == nil || !strings.Contains(err.Error(), "同名文件") {
		t.Fatalf("error=%v", err)
	}
}

func TestFamilyMoveWithinSameDirectoryIsNoop(t *testing.T) {
	family := newTestFamilyAPI(t, "http://127.0.0.1:1")
	target := &folder{FileID: json.Number("8"), DirName: "dir"}
	source := &fileInfo{FileID: json.Number("7"), ParentFileID: json.Number("8"), FileName: "a.txt"}
	if err := family.Move(context.Background(), target, source); err != nil {
		t.Fatal(err)
	}
}

func TestFamilyTaskStates(t *testing.T) {
	for _, status := range []int{taskQueued, taskRunning} {
		if !taskPending(status) {
			t.Fatalf("status %d should be pending", status)
		}
	}
	for _, status := range []int{0, taskConflict, taskDone, 5} {
		if taskPending(status) {
			t.Fatalf("status %d should not be pending", status)
		}
	}
}

func TestFamilyTaskResult(t *testing.T) {
	tests := []struct {
		name    string
		result  batchTaskResponse
		wantErr bool
	}{
		{name: "complete", result: batchTaskResponse{SubTaskCount: 3, SucceededCount: 3}},
		{name: "failed", result: batchTaskResponse{SubTaskCount: 3, SucceededCount: 2, FailedCount: 1}, wantErr: true},
		{name: "skipped", result: batchTaskResponse{SubTaskCount: 3, SucceededCount: 2, SkipCount: 1}, wantErr: true},
		{name: "incomplete", result: batchTaskResponse{SubTaskCount: 3, SucceededCount: 2}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.result.resultError()
			if (err != nil) != test.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, test.wantErr)
			}
		})
	}
}

func TestPersonalToFamilyTransferProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/batch/createBatchTask.action":
			if r.Header.Get("SessionKey") != "personal-key" {
				t.Errorf("create SessionKey = %q", r.Header.Get("SessionKey"))
			}
			if r.Form.Get("copyType") != "1" || r.Form.Get("familyId") != "123" || r.Form.Get("targetFolderId") != "10" {
				t.Errorf("create form = %v", r.Form)
			}
			if strings.Contains(r.Form.Get("taskInfos"), "srcParentId") {
				t.Errorf("personal taskInfos contains srcParentId: %s", r.Form.Get("taskInfos"))
			}
			_, _ = io.WriteString(w, `{"res_code":0,"taskId":"transfer-1"}`)
		case "/batch/checkBatchTask.action":
			if r.Header.Get("SessionKey") != "personal-key" || r.URL.Query().Get("familyId") != "" {
				t.Errorf("poll SessionKey=%q query=%v", r.Header.Get("SessionKey"), r.URL.Query())
			}
			_, _ = io.WriteString(w, `{"res_code":0,"taskStatus":4,"successedCount":1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	family := newTestFamilyAPI(t, server.URL)
	target := &folder{FileID: json.Number("10"), DirName: "target"}
	source := &fileInfo{FileID: json.Number("7"), ParentFileID: json.Number("-11"), FileName: "a.txt"}
	if err := family.CopyFromPersonal(context.Background(), target, source); err != nil {
		t.Fatal(err)
	}
}

func TestFamilyToPersonalTransferProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/batch/createBatchTask.action":
			if r.Header.Get("SessionKey") != "family-key" {
				t.Errorf("create SessionKey = %q", r.Header.Get("SessionKey"))
			}
			if r.Form.Get("copyType") != "2" || r.Form.Get("familyId") != "123" || r.Form.Get("targetFolderId") != "-11" {
				t.Errorf("create form = %v", r.Form)
			}
			if !strings.Contains(r.Form.Get("taskInfos"), `"srcParentId":"8"`) {
				t.Errorf("family taskInfos missing srcParentId: %s", r.Form.Get("taskInfos"))
			}
			_, _ = io.WriteString(w, `{"res_code":0,"taskId":"transfer-2"}`)
		case "/batch/checkBatchTask.action":
			if r.Header.Get("SessionKey") != "family-key" || r.URL.Query().Get("familyId") != "123" {
				t.Errorf("poll SessionKey=%q query=%v", r.Header.Get("SessionKey"), r.URL.Query())
			}
			_, _ = io.WriteString(w, `{"res_code":0,"taskStatus":4,"successedCount":1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	family := newTestFamilyAPI(t, server.URL)
	target := &folder{FileID: json.Number("-11"), DirName: "personal"}
	source := &fileInfo{FileID: json.Number("7"), ParentFileID: json.Number("8"), FileName: "a.txt"}
	if err := family.CopyToPersonal(context.Background(), target, source); err != nil {
		t.Fatal(err)
	}
}
