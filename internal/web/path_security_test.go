package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/engine"
)

func testWebServer(t *testing.T) *Server {
	t.Helper()
	_, f, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(f), "..", "..", "testdata", "example")
	p, pdb, _, err := engine.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDocFileServesLedgerDocs(t *testing.T) {
	s := testWebServer(t)
	req := httptest.NewRequest(http.MethodGet, "/docfile/personal/docs/by-account/Assets/BR/Alfa/ContaCorrente/20240301_statement.txt", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() == 0 {
		t.Fatal("expected non-empty doc body")
	}
}

func TestDocFileRejectsNonDocsPath(t *testing.T) {
	s := testWebServer(t)
	req := httptest.NewRequest(http.MethodGet, "/docfile/personal/main.beancount", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status %d want 404", rr.Code)
	}
}

func TestDocFileRejectsEncodedTraversal(t *testing.T) {
	s := testWebServer(t)
	// ServeMux leaves %2e%2e literal; path.Clean + IsLedgerDocPath / OpenRoot must reject.
	req := httptest.NewRequest(http.MethodGet, "/docfile/personal/docs/by-account/%2e%2e/%2e%2e/main.beancount", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status %d want 404 body=%s", rr.Code, rr.Body.String())
	}
	body, _ := io.ReadAll(rr.Body)
	if strings.Contains(string(body), "include") || strings.Contains(string(body), "option") {
		t.Fatal("traversal leaked ledger file content")
	}
}

func TestAccountInvalidPathEncoding(t *testing.T) {
	s := testWebServer(t)
	// httptest.NewRequest rejects invalid escapes; craft URL.Path with a bad percent sequence.
	req := &http.Request{
		Method:     http.MethodGet,
		URL:        &url.URL{Path: "/l/personal/account/foo%zz", RawPath: "/l/personal/account/foo%zz"},
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d want 400 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid account path encoding") {
		t.Fatalf("body=%q", rr.Body.String())
	}
}

func TestCommodityInvalidPathEncoding(t *testing.T) {
	s := testWebServer(t)
	req := &http.Request{
		Method:     http.MethodGet,
		URL:        &url.URL{Path: "/l/personal/commodity/foo%zz", RawPath: "/l/personal/commodity/foo%zz"},
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d want 400 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid commodity path encoding") {
		t.Fatalf("body=%q", rr.Body.String())
	}
}
