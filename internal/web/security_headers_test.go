package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/contapila-go/pkg/project"
)

func TestSecurityHeadersOnIndex(t *testing.T) {
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
	p, err := project.OpenProject(root)
	if err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	s, err := New(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()[:min(200, rr.Body.Len())])
	}
	h := rr.Header()
	if h.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("X-Content-Type-Options=%q", h.Get("X-Content-Type-Options"))
	}
	if h.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("X-Frame-Options=%q", h.Get("X-Frame-Options"))
	}
	if csp := h.Get("Content-Security-Policy"); csp == "" {
		t.Fatal("missing CSP")
	}
	if h.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("Referrer-Policy=%q", h.Get("Referrer-Policy"))
	}
}
