package web_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/web"

	_ "github.com/lucasew/contapila-go/internal/plugins/queries"
)

func TestQuerySidebarAndDetail(t *testing.T) {
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

	// List page (plugin).
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/queries", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("queries list: status %d body=%s", rr.Code, rr.Body.String()[:min(200, rr.Body.Len())])
	}
	body := rr.Body.String()
	if !strings.Contains(body, "cash") {
		t.Fatalf("queries list missing cash: %s", body[:min(400, len(body))])
	}
	// Sidebar section on any ledger page.
	if !strings.Contains(body, "Queries") {
		t.Fatalf("expected Queries sidebar section")
	}

	// Detail page for named query from surface.beancount.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/query/cash", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("query detail: status %d", rr.Code)
	}
	body = rr.Body.String()
	if !strings.Contains(body, "account") {
		t.Fatalf("detail missing BQL text: %s", truncateBody(body, 500))
	}
	// Server-side chroma via shared codeHighlight (bql → SQL lexer).
	if !strings.Contains(body, "code-hl") && !strings.Contains(body, "class=\"hl-") {
		t.Fatalf("expected chroma code highlight classes: %s", truncateBody(body, 400))
	}
	if !strings.Contains(body, "not available yet") {
		t.Fatalf("detail missing placeholder: %s", truncateBody(body, 500))
	}

	// Unknown name still 200 with error banner (or we could 404 — keep soft).
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/query/nope", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("missing query: status %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "query not found") {
		t.Fatalf("expected not found message")
	}
}
