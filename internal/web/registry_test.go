package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/engine"
)

func exampleRoot(t *testing.T) string {
	t.Helper()
	// testdata/example lives at repo root relative to this package.
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "example"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDefaultRegistryMountParity(t *testing.T) {
	root := exampleRoot(t)
	p, pdb, _, err := engine.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()

	// Smoke matrix: same URLs the live UI and build expand for default filters.
	paths := []struct {
		path string
		code int
	}{
		{"/", http.StatusOK},
		{"/static/app.css", http.StatusOK},
		{"/l/personal/check", http.StatusOK},
		{"/l/personal/balances", http.StatusOK},
		{"/l/personal/journal", http.StatusOK},
		{"/l/personal/pnl", http.StatusOK},
		{"/l/personal/networth", http.StatusOK},
		{"/l/personal/documents", http.StatusOK},
		{"/l/personal/prices", http.StatusOK},
		{"/l/personal/", http.StatusFound},
		{"/l/personal/account/Assets:BR:Alfa:ContaCorrente", http.StatusOK},
		{"/l/personal/commodity/USD", http.StatusOK},
		{"/l/personal/no-such-page", http.StatusNotFound},
	}
	for _, tc := range paths {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rr.Code != tc.code {
			t.Errorf("%s: status %d want %d body=%s", tc.path, rr.Code, tc.code, truncate(rr.Body.String(), 120))
		}
	}
}

func TestRegistryInstancesExample(t *testing.T) {
	root := exampleRoot(t)
	ctx, err := NewSiteCtx(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(ctx.Project, ctx.Prices)
	if err != nil {
		t.Fatal(err)
	}
	reg := DefaultRegistry(s)
	insts, err := reg.Instances(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(insts) == 0 {
		t.Fatal("expected instances")
	}

	byPath := map[string]Kind{}
	for _, inst := range insts {
		if _, dup := byPath[inst.Path]; dup {
			t.Errorf("duplicate path %q", inst.Path)
		}
		byPath[inst.Path] = inst.Kind
	}

	wantPages := []string{
		"/",
		"/l/personal/",
		"/l/acme/",
		"/l/personal/check",
		"/l/personal/pnl",
		"/l/personal/networth",
		"/l/acme/check",
	}
	for _, p := range wantPages {
		if k, ok := byPath[p]; !ok || k != KindPage {
			t.Errorf("missing page %q (ok=%v kind=%v)", p, ok, k)
		}
	}
	if _, ok := byPath["/static/app.css"]; !ok {
		t.Error("missing /static/app.css")
	}
	// Account + commodity expansion from engine maps.
	var accounts, commodities int
	for p, k := range byPath {
		if k != KindPage {
			continue
		}
		if strings.Contains(p, "/account/") {
			accounts++
		}
		if strings.Contains(p, "/commodity/") {
			commodities++
		}
	}
	if accounts == 0 {
		t.Error("expected account pages")
	}
	if commodities == 0 {
		t.Error("expected commodity pages")
	}
}

func TestFileRel(t *testing.T) {
	tests := []struct {
		inst Instance
		want string
	}{
		{Instance{Path: "/", Kind: KindPage}, "index.html"},
		{Instance{Path: "/l/personal/", Kind: KindPage}, "l/personal/index.html"},
		{Instance{Path: "/l/personal/check", Kind: KindPage}, "l/personal/check/index.html"},
		{Instance{Path: "/static/app.css", Kind: KindStatic}, "static/app.css"},
		{Instance{Path: "/docfile/personal/docs/README.md", Kind: KindDoc}, "docfile/personal/docs/README.md"},
	}
	for _, tc := range tests {
		got, err := fileRel(tc.inst)
		if err != nil {
			t.Errorf("%s: %v", tc.inst.Path, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %q want %q", tc.inst.Path, got, tc.want)
		}
	}
}

func TestReportPages(t *testing.T) {
	pages := ReportPages()
	if len(pages) != 7 {
		t.Fatalf("got %d pages: %v", len(pages), pages)
	}
	if pages[0] != "check" || pages[len(pages)-1] != "prices" {
		t.Fatalf("unexpected order: %v", pages)
	}
}
