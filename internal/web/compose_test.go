package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/a-h/templ"

	"github.com/lucasew/contapila-go/internal/engine"
)

func TestCloneDoesNotMutateDefault(t *testing.T) {
	base := DefaultPages()
	n := len(base.BuildIDs())
	cl := base.Clone()
	cl.Register(Page{
		ID: "extra-clone", Label: "X", Order: 1, Sidebar: true, Build: true,
		Body: func(d PageData) templ.Component { return templ.NopComponent },
	})
	if len(DefaultPages().BuildIDs()) != n {
		t.Fatal("DefaultPages mutated by Clone+Register")
	}
	if _, ok := cl.Lookup("extra-clone"); !ok {
		t.Fatal("clone missing extra")
	}
	if _, ok := DefaultPages().Lookup("extra-clone"); ok {
		t.Fatal("default should not have extra")
	}
}

func TestComposePages(t *testing.T) {
	reg := ComposePages(DefaultPages(), Page{
		ID: "extra-compose", Label: "Extra", Order: 99, Sidebar: true, Build: true,
		Body: func(d PageData) templ.Component { return templ.NopComponent },
	})
	if _, ok := reg.Lookup("check"); !ok {
		t.Fatal("missing builtin check")
	}
	if _, ok := reg.Lookup("extra-compose"); !ok {
		t.Fatal("missing extra")
	}
	if _, ok := DefaultPages().Lookup("extra-compose"); ok {
		t.Fatal("default polluted")
	}
}

func TestContributePageAndReportPages(t *testing.T) {
	resetContributedPagesForTest()
	t.Cleanup(resetContributedPagesForTest)

	ContributePage(Page{
		ID: "contrib-a", Label: "A", Order: 1, Sidebar: true, Build: true,
		Body: func(d PageData) templ.Component { return templ.NopComponent },
	})
	ids := ReportPages()
	found := false
	for _, id := range ids {
		if id == "contrib-a" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ReportPages missing contrib: %v", ids)
	}
	// DefaultPages still builtins only.
	if _, ok := DefaultPages().Lookup("contrib-a"); ok {
		t.Fatal("contrib should not be on DefaultPages singleton")
	}
}

func TestFilterPagesByPlugins(t *testing.T) {
	reg := ComposePages(DefaultPages())
	filtered := FilterPagesByPlugins(reg, func(key string) bool {
		return key != "web/prices"
	})
	if _, ok := filtered.Lookup("prices"); ok {
		t.Fatal("prices should be filtered out")
	}
	if _, ok := filtered.Lookup("check"); !ok {
		t.Fatal("check should remain")
	}
	side := filtered.Sidebar()
	for _, p := range side {
		if p.ID == "prices" {
			t.Fatal("prices still in sidebar")
		}
	}
}

func TestPagePluginKey(t *testing.T) {
	if PagePluginKey("accounts") != "web/accounts" {
		t.Fatal(PagePluginKey("accounts"))
	}
}

func TestResolvedPagesRespectsCUEPlugins(t *testing.T) {
	resetContributedPagesForTest()
	t.Cleanup(resetContributedPagesForTest)
	ContributePage(Page{
		ID: "accounts", Label: "Accounts", Order: 25, Sidebar: true, Build: true,
		Body: func(d PageData) templ.Component { return templ.NopComponent },
	})

	dir := t.TempDir()
	// Minimal project: contapila.cue disables web/accounts + one ledger.
	if err := os.WriteFile(filepath.Join(dir, "contapila.cue"), []byte(`
plugins: {
	"web/accounts": { enabled: false }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	ledgerDir := filepath.Join(dir, "personal")
	if err := os.Mkdir(ledgerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledgerDir, "main.beancount"), []byte(`
option "operating_currency" "BRL"
2020-01-01 open Assets:Cash BRL
`), 0o644); err != nil {
		t.Fatal(err)
	}

	p, pdb, _, err := engine.OpenProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	// Dynamic registry (s.Pages nil).
	sess := NewSession(dir)
	reg := s.resolvedPages(sess)
	if _, ok := reg.Lookup("accounts"); ok {
		t.Fatal("accounts should be disabled by CUE")
	}
	if _, ok := reg.Lookup("check"); !ok {
		t.Fatal("check should remain")
	}

	h := s.Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/accounts", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("accounts status %d want 404", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/check", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("check status %d", rr.Code)
	}
}
