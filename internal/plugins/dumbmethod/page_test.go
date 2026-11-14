package dumbmethod_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/plugins/dumbmethod"
	"github.com/lucasew/contapila-go/internal/web"
)

func TestPageRegistered(t *testing.T) {
	web.BindPluginPages()
	found := false
	for _, id := range web.ReportPages() {
		if id == dumbmethod.PageID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%s not in ReportPages after BindPluginPages", dumbmethod.PageID)
	}
}

func TestPageLive(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("testdata", "project"))
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
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/dumb_method?amount=1000", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rr.Code, truncate(rr.Body.String(), 300))
	}
	body := rr.Body.String()
	for _, want := range []string{
		"Contributions",
		"Classification",
		"data-pnl-tree",
		"Assets:BR:B3:Acao", // holding row nested under its class/asset
		"PETR4",
		"MXRF11",
		"868.00", // 28 whole PETR4 units at 31.00
		"126.00", // 12 whole MXRF11 units at 10.50
		"Excluded",
		"Unclassified",
		"Assets:BR:Cash",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
