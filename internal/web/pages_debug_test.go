package web_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/engine"
	_ "github.com/lucasew/contapila-go/internal/plugins/accountslist"
	_ "github.com/lucasew/contapila-go/internal/plugins/checkclosing"
	"github.com/lucasew/contapila-go/internal/web"
)

func TestDebugPageSmoke(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "example"))
	if err != nil {
		t.Fatal(err)
	}
	p, pdb, _, err := engine.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := web.New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/debug", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rr.Code, truncateBody(rr.Body.String(), 200))
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Plugins") {
		t.Fatal("missing Plugins section")
	}
	if !strings.Contains(body, "web_accounts") {
		t.Fatalf("expected web_accounts in body: %s", truncateBody(body, 500))
	}
	if !strings.Contains(body, "check_closing") {
		t.Fatalf("expected check_closing in body: %s", truncateBody(body, 500))
	}
	if !strings.Contains(body, "Full config") {
		t.Fatal("missing Full config section")
	}
	// Server-side chroma CUE highlighting.
	if !strings.Contains(body, "cue-hl") && !strings.Contains(body, "class=\"cue-") {
		t.Fatalf("expected chroma CUE highlight classes: %s", truncateBody(body, 400))
	}
}

func truncateBody(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
