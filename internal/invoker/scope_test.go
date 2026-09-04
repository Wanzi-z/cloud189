package invoker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestScopeSurvivesRetry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"errorCode":"InvalidSessionKey","errorMsg":"expired"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	invoker := NewInvoker(server.URL, func(context.Context) error { return nil }, &Config{})
	prepared := 0
	invoker.SetPrepareE(func(req *http.Request) error {
		prepared++
		if RequestScope(req) != FamilyScope {
			t.Fatalf("request scope = %v, want FamilyScope", RequestScope(req))
		}
		return nil
	})
	var response map[string]any
	if err := invoker.GetScoped(FamilyScope, "/retry", nil, &response); err != nil {
		t.Fatal(err)
	}
	if prepared != 2 || requests != 2 {
		t.Fatalf("prepared=%d requests=%d, want 2/2", prepared, requests)
	}
}

func TestRetryRefreshStopsOnCancellation(t *testing.T) {
	served := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errorCode":"InvalidSessionKey","errorMsg":"expired"}`))
		close(served)
	}))
	defer server.Close()
	refreshed := false
	invoker := NewInvoker(server.URL, func(context.Context) error {
		refreshed = true
		return nil
	}, &Config{})
	ctx, cancel := context.WithCancel(context.Background())
	started := time.Now()
	done := make(chan error, 1)
	go func() {
		done <- invoker.GetContext(ctx, "/retry", nil, &map[string]any{})
	}()
	<-served
	time.Sleep(20 * time.Millisecond)
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if refreshed {
		t.Fatal("refresh ran after cancellation")
	}
	if time.Since(started) >= 200*time.Millisecond {
		t.Fatal("canceled retry waited for backoff")
	}
}

func TestPrepareErrorStopsRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()

	want := errors.New("prepare failed")
	invoker := NewInvoker(server.URL, func(context.Context) error { return nil }, &Config{})
	invoker.SetPrepareE(func(req *http.Request) error { return want })
	err := invoker.Get("/blocked", nil, &map[string]any{})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if requests != 0 {
		t.Fatalf("server received %d requests, want 0", requests)
	}
}
