package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/etkecc/go-kit/httpclient"

	"github.com/etkecc/bunny-upload/internal/config"
)

// TestUploadFileRewindsOnRetry: no GetBody meant a retried PUT of an *os.File failed with ErrNonReplayableBody.
func TestUploadFileRewindsOnRetry(t *testing.T) {
	content := []byte("bunny upload payload, has to survive a rewind")

	path := filepath.Join(t.TempDir(), "asset.txt")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	var attempts atomic.Int32
	var lastBody atomic.Value
	var lastLen atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lastBody.Store(body)
		lastLen.Store(r.ContentLength) // -1 here means the upload went chunked (the bug)
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable) // force exactly one retry
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client = httpclient.NewSingleHost()
	cfg = &config.Config{}
	cfg.Storage.Password = "test-key"

	if err := uploadFile(srv.URL+"/asset.txt", path, "asset.txt"); err != nil {
		t.Fatalf("uploadFile returned error: %v", err)
	}

	if got := attempts.Load(); got != 2 {
		t.Fatalf("want 2 attempts (503 then retry), got %d", got)
	}
	if got, _ := lastBody.Load().([]byte); !bytes.Equal(got, content) {
		t.Fatalf("replayed body mismatch: got %q want %q", got, content)
	}
	if got := lastLen.Load(); got != int64(len(content)) {
		t.Fatalf("want fixed Content-Length %d, got %d (-1 = chunked)", len(content), got)
	}
}
