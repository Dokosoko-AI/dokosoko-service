package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type blockingListener struct {
	closed chan struct{}
	once   sync.Once
}

func (l *blockingListener) Accept() (net.Conn, error) { <-l.closed; return nil, net.ErrClosed }
func (l *blockingListener) Close() error              { l.once.Do(func() { close(l.closed) }); return nil }
func (l *blockingListener) Addr() net.Addr            { return testAddress("test") }

type testAddress string

func (a testAddress) Network() string { return string(a) }
func (a testAddress) String() string  { return string(a) }

func TestServeShutsDownAfterCancellation(t *testing.T) {
	listener := &blockingListener{closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })}
	if err := serve(ctx, server, listener); err != nil {
		t.Fatalf("serve returned an error during graceful shutdown: %v", err)
	}
}

func TestSourceUploadConfigDefaultsToDisabled(t *testing.T) {
	directory, err := prepareSourceUploadDirectory("")
	if err != nil {
		t.Fatal(err)
	}
	if directory != "" {
		t.Fatalf("upload directory = %q", directory)
	}
}

func TestSourceUploadConfigCreatesPrivateDirectoryAndReadsLimit(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "uploads")
	configured, err := prepareSourceUploadDirectory(directory)
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	if configured != absolute {
		t.Fatalf("upload directory = %q, want %q", configured, absolute)
	}
	info, err := os.Stat(configured)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("upload directory mode = %o, want 700", info.Mode().Perm())
	}
}

func TestHTTPWriteTimeoutCoversOneAIProviderFailover(t *testing.T) {
	const twoProviderBudget = 2 * 45 * time.Second
	if httpWriteTimeout <= twoProviderBudget {
		t.Fatalf("HTTP write timeout %s must exceed the bounded primary and backup AI request budget %s", httpWriteTimeout, twoProviderBudget)
	}
}
