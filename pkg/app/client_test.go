package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	if err := client.remove(context.Background(), entry); err == nil {
		t.Fatal("remove returned nil after server failure")
	}
}

func TestPersonalCopyAggregatesErrors(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client := newTestFamilyAPI(t, server.URL).base
	target := &folder{FileID: "10", DirName: "target"}
	first := &fileInfo{FileID: "1", FileName: "first"}
	second := &fileInfo{FileID: "2", FileName: "second"}
	if err := client.copyEntries(context.Background(), target, first, second); err == nil {
		t.Fatal("copyEntries discarded an earlier error")
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
