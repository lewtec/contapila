package web_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/engine"
	_ "github.com/lucasew/contapila-go/internal/plugins/accountslist"
	"github.com/lucasew/contapila-go/internal/web"
)

func TestPluginsAndConfigPagesSmoke(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "example"))
	if err != nil {
		t.Fatal(err)
	}
	p, pdb, _, err := engine.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := web.New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/plugins", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("plugins status %d body=%s", rr.Code, truncateBody(rr.Body.String(), 200))
	}
	body := rr.Body.String()
	if !strings.Contains(body, "web_accounts") {
		t.Fatalf("expected web_accounts in body: %s", truncateBody(body, 500))
	}
	if !strings.Contains(body, "check_closing") {
		t.Fatalf("expected check_closing in body: %s", truncateBody(body, 500))
	}
	if !strings.Contains(body, "code-hl") && !strings.Contains(body, "class=\"hl-") {
		t.Fatalf("expected chroma code highlight classes: %s", truncateBody(body, 400))
	}
	// Sidebar Debug section labels.
	if !strings.Contains(body, "Plugins") || !strings.Contains(body, "Config") {
		t.Fatalf("expected Debug section nav Labels Plugins/Config: %s", truncateBody(body, 400))
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/config", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("config status %d", rr.Code)
	}
	body = rr.Body.String()
	if !strings.Contains(body, "Prelude") {
		t.Fatal("missing config blurb")
	}
	if !strings.Contains(body, "code-hl") && !strings.Contains(body, "class=\"hl-") {
		t.Fatalf("expected chroma on config: %s", truncateBody(body, 400))
	}

	// Old /debug slug is gone.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/debug", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("debug status %d want 404", rr.Code)
	}
}

func truncateBody(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
