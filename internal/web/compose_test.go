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

func TestPluginScopePageAndReportPages(t *testing.T) {
	resetPluginPagesForTest()
	t.Cleanup(resetPluginPagesForTest)

	// Simulate Reg.Page path: host middleman on package-local registry.
	s := &scope{moduleID: "web_contrib_a"}
	s.Page(Page{
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
	if _, ok := DefaultPages().Lookup("contrib-a"); ok {
		t.Fatal("contrib should not be on DefaultPages singleton")
	}
	pages := pluginContributedPages()
	foundKey := false
	for _, p := range pages {
		if p.ID == "contrib-a" && p.PluginKey == "web_contrib_a" {
			foundKey = true
		}
	}
	if !foundKey {
		t.Fatalf("plugin key tag missing: %+v", pages)
	}
}

func TestFilterContribPages(t *testing.T) {
	pages := []Page{
		{ID: "on-default", DefaultEnabled: true},
		{ID: "off-default", DefaultEnabled: false},
		{ID: "forced-off", DefaultEnabled: true},
		{ID: "forced-on", DefaultEnabled: false},
	}
	got := filterContribPages(pages, map[string]bool{
		"web_forced-off": false, // PagePluginKey("forced-off")
		"web_forced-on":  true,
	}, nil)
	ids := map[string]bool{}
	for _, p := range got {
		ids[p.ID] = true
	}
	if !ids["on-default"] || ids["off-default"] || ids["forced-off"] || !ids["forced-on"] {
		t.Fatalf("got %v", ids)
	}
	// Nil flags → only DefaultEnabled.
	got = filterContribPages(pages, nil, nil)
	ids = map[string]bool{}
	for _, p := range got {
		ids[p.ID] = true
	}
	if !ids["on-default"] || !ids["forced-off"] || ids["off-default"] || ids["forced-on"] {
		t.Fatalf("nil flags: %v", ids)
	}
}

func TestPagePluginKey(t *testing.T) {
	if PagePluginKey("accounts") != "web_accounts" {
		t.Fatal(PagePluginKey("accounts"))
	}
}

func TestPagePluginKeyResolver(t *testing.T) {
	if got := pagePluginKey(Page{ID: "accounts"}); got != "web_accounts" {
		t.Fatalf("empty PluginKey: %q", got)
	}
	if got := pagePluginKey(Page{ID: "accounts", PluginKey: "custom"}); got != "custom" {
		t.Fatalf("explicit PluginKey: %q", got)
	}
}

func TestResolvedPagesRespectsCUEPlugins(t *testing.T) {
	resetPluginPagesForTest()
	t.Cleanup(resetPluginPagesForTest)
	(&scope{moduleID: "web_accounts"}).Page(Page{
		ID: "accounts", Label: "Accounts", Order: 25, Sidebar: true, Build: true,
		Body: func(d PageData) templ.Component { return templ.NopComponent },
	})

	writeProj := func(t *testing.T, cue string) string {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "contapila.cue"), []byte(cue), 0o644); err != nil {
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
		return dir
	}

	t.Run("opt-in off by default", func(t *testing.T) {
		dir := writeProj(t, "// empty\n")
		p, pdb, _, err := engine.OpenProject(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		s, err := New(p, pdb)
		if err != nil {
			t.Fatal(err)
		}
		reg := s.resolvedPages(t.Context(), NewSession(dir))
		if _, ok := reg.Lookup("accounts"); ok {
			t.Fatal("accounts should be off without plugins stanza")
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
	})

	t.Run("explicit enable", func(t *testing.T) {
		dir := writeProj(t, `plugins: { "web_accounts": { enabled: true } }`)
		p, pdb, _, err := engine.OpenProject(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		s, err := New(p, pdb)
		if err != nil {
			t.Fatal(err)
		}
		reg := s.resolvedPages(t.Context(), NewSession(dir))
		if _, ok := reg.Lookup("accounts"); !ok {
			t.Fatal("accounts should be on when enabled: true")
		}
		h := s.Handler()
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/check", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("check status %d", rr.Code)
		}
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/accounts", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("accounts status %d want 200", rr.Code)
		}
	})
}
