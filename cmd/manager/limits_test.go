package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLimitMCPRejectsLargeContentLength(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("oversized request reached downstream handler")
	})
	h := limitMCP(1, 4, next)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("12345"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestLimitMCPRejectsConcurrentOverflow(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	h := limitMCP(1, 1024, next)
	server := httptest.NewServer(h)
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		resp, err := http.Get(server.URL)
		if err == nil {
			_ = resp.Body.Close()
		}
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first request did not enter handler")
	}

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWithRequestIDOverridesClientValue(t *testing.T) {
	var seen string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-CodeBridge-Request-ID")
		w.WriteHeader(http.StatusNoContent)
	})
	h := withRequestID(next)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("X-CodeBridge-Request-ID", "client-controlled")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if seen == "" || seen == "client-controlled" {
		t.Fatalf("request id was not regenerated: %q", seen)
	}
	if got := rec.Header().Get("X-Request-ID"); got != seen {
		t.Fatalf("response request id = %q, handler saw %q", got, seen)
	}
}

func TestLimitAgentConnectionsRejectsOverflow(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	h := limitAgentConnections(1, next)
	server := httptest.NewServer(h)
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		resp, err := http.Get(server.URL)
		if err == nil {
			_ = resp.Body.Close()
		}
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first agent request did not enter handler")
	}

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
	if got := resp.Header.Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
