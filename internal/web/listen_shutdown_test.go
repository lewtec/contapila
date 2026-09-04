package web

import (
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/pkg/project"
)

func TestListenGracefulShutdownOnSIGTERM(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "contapila.cue"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := filepath.Join(root, "personal")
	if err := os.MkdirAll(ledger, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledger, "main.beancount"), []byte("2020-01-01 open Assets:Cash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := project.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatalf("OpenProject: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- Listen(t.Context(), p, nil, "", addr)
	}()

	var last error
	ready := false
	for i := 0; i < 100; i++ {
		resp, err := http.Get("http://" + addr + "/")
		if err == nil {
			if _, copyErr := io.Copy(io.Discard, resp.Body); copyErr != nil {
				t.Logf("drain body: %v", copyErr)
			}
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Logf("close body: %v", closeErr)
			}
			ready = true
			break
		}
		last = err
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("server never became ready: last=%v", last)
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Listen returned error: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("Listen did not return after SIGTERM")
	}
}
