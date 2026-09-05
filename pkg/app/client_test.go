package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenPropagatesConfigErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := Open(path)
	if err == nil || client != nil {
		t.Fatalf("New() = %v, %v; want nil client and config error", client, err)
	}
}

func TestPersonalDeleteReturnsTransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client := newTestFamilyAPI(t, server.URL).base
	entry := &fileInfo{FileID: "1", FileName: "file"}
	if err := client.Remove(context.Background(), entry); err == nil {
		t.Fatal("remove returned nil after server failure")
	}
}

func TestPersonalBatchCopyRejectsMissingTarget(t *testing.T) {
	client := newTestFamilyAPI(t, "http://127.0.0.1:1").base
	first := &fileInfo{FileID: "1", FileName: "first"}
	if err := client.Copy(context.Background(), nil, first); err == nil {
		t.Fatal("copy accepted nil target")
	}
}

func TestPersonalBatchTaskProtocol(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("SessionKey") != "personal-key" {
			t.Errorf("SessionKey = %q", r.Header.Get("SessionKey"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/batch/createBatchTask.action":
			if r.Form.Get("type") != "COPY" || r.Form.Get("targetFolderId") != "10" {
				t.Errorf("create form = %v", r.Form)
			}
			if r.Form.Get("familyId") != "" {
				t.Errorf("personal task must not carry familyId: %v", r.Form)
			}
			if !strings.Contains(r.Form.Get("taskInfos"), `"srcParentId":"8"`) {
				t.Errorf("taskInfos missing srcParentId: %s", r.Form.Get("taskInfos"))
			}
			_, _ = io.WriteString(w, `{"res_code":0,"taskId":"personal-task-1"}`)
		case "/batch/checkBatchTask.action":
			if r.URL.Query().Get("familyId") != "" {
				t.Errorf("personal poll must not carry familyId: %v", r.URL.Query())
			}
			if r.Form.Get("taskId") != "personal-task-1" || r.Form.Get("type") != "COPY" {
				t.Errorf("poll form = %v", r.Form)
			}
			_, _ = io.WriteString(w, `{"res_code":0,"taskStatus":4,"subTaskCount":1,"successedCount":1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newTestFamilyAPI(t, server.URL).base
	target := &folder{FileID: "10", DirName: "target"}
	source := &fileInfo{FileID: "7", ParentFileID: json.Number("8"), FileName: "a.txt"}
	if err := client.Copy(context.Background(), target, source); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths, ","); got != "/batch/createBatchTask.action,/batch/checkBatchTask.action" {
		t.Fatalf("paths=%s", got)
	}
}

func TestPersonalMutationPropagatesCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte(`{}`))
	}))
	defer func() {
		close(release)
		server.Close()
	}()
	client := newTestFamilyAPI(t, server.URL).base
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- client.rename(ctx, &fileInfo{FileID: "1", FileName: "old"}, "new")
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("rename error = %v, want context cancellation", err)
	}
}

func TestOpenReturnsExportedClient(t *testing.T) {
	client, err := Open(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var exported *Client = client
	if exported == nil {
		t.Fatal("New returned nil client")
	}
}
