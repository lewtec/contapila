package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a-h/templ"

	"github.com/lucasew/contapila-go/internal/engine"
)

func TestDefaultPagesSidebarAndBuild(t *testing.T) {
	resetPluginPagesForTest()
	t.Cleanup(resetPluginPagesForTest)

	r := DefaultPages()
	side := r.Sidebar()
	if len(side) != 9 {
		t.Fatalf("sidebar len %d want 9: %v", len(side), ids(side))
	}
	if side[0].ID != "check" || side[len(side)-1].ID != "config" {
		t.Fatalf("sidebar order: %v", ids(side))
	}
	build := r.BuildIDs()
	if len(build) != 9 || build[0] != "check" || build[len(build)-1] != "config" {
		t.Fatalf("build ids: %v", build)
	}
	// ReportPages is builtins then any bound plugin pages.
	rp := ReportPages()
	if len(rp) < len(build) {
		t.Fatalf("ReportPages %v shorter than builtins %v", rp, build)
	}
	for i := range build {
		if rp[i] != build[i] {
			t.Fatalf("ReportPages builtins prefix: %v want start %v", rp, build)
		}
	}
}

func TestPageRegistryRegisterAndLookup(t *testing.T) {
	r := NewPageRegistry()
	r.Register(Page{
		ID: "extra", Label: "Extra", Order: 5, Sidebar: true, Build: true,
		Body: func(d PageData) templ.Component { return templ.Raw("extra-body") },
	})
	p, ok := r.Lookup("extra")
	if !ok || p.Label != "Extra" {
		t.Fatalf("lookup: ok=%v page=%+v", ok, p)
	}
	if _, ok := r.Lookup("missing"); ok {
		t.Fatal("expected missing")
	}
	side := r.Sidebar()
	if len(side) != 1 || side[0].ID != "extra" {
		t.Fatalf("sidebar: %v", ids(side))
	}
}

func TestSidebarSectionsOrderAndMerge(t *testing.T) {
	r := NewPageRegistry()
	body := func(d PageData) templ.Component { return templ.NopComponent }
	r.Register(Page{ID: "check", Label: "Check", Order: 10, Sidebar: true, Body: body})
	r.Register(Page{ID: "qlist", Label: "All", Order: 1, Section: "Queries", Sidebar: false, Body: body})
	r.Register(Page{ID: "events", Label: "Events", Order: 20, Section: "Activity", Sidebar: true, Body: body})
	// Second plugin-style page into same section as qlist (first-seen keeps Queries order).
	r.Register(Page{ID: "qextra", Label: "Extra Q", Order: 2, Section: "Queries", Sidebar: true, Body: body})
	r.Register(Page{ID: "balances", Label: "Balances", Order: 20, Sidebar: true, Body: body})

	secs := r.SidebarSections()
	// Reports first (check, balances by Order), then Queries (qextra only — Sidebar pages),
	// then Activity — registration order of first non-Reports section was Queries then Activity.
	if len(secs) != 3 {
		t.Fatalf("sections %d: %+v", len(secs), secs)
	}
	if secs[0].Title != DefaultSidebarSection {
		t.Fatalf("first section %q want %q", secs[0].Title, DefaultSidebarSection)
	}
	if ids(secs[0].Pages)[0] != "check" || ids(secs[0].Pages)[1] != "balances" {
		t.Fatalf("Reports pages: %v", ids(secs[0].Pages))
	}
	if secs[1].Title != "Queries" || ids(secs[1].Pages)[0] != "qextra" {
		t.Fatalf("Queries section: %+v", secs[1])
	}
	if secs[2].Title != "Activity" || ids(secs[2].Pages)[0] != "events" {
		t.Fatalf("Activity section: %+v", secs[2])
	}
}

func TestSidebarNavSectionsQueryAttach(t *testing.T) {
	r := NewPageRegistry()
	body := func(d PageData) templ.Component { return templ.NopComponent }
	r.Register(Page{ID: "check", Label: "Check", Order: 10, Sidebar: true, Body: body})
	r.Register(Page{ID: "queries", Label: "Queries", Order: 65, Section: "Queries", Sidebar: false, Body: body})

	d := PageData{
		Pages:    r,
		QueryNav: []QueryNavItem{{Name: "cash"}, {Name: "ar"}},
	}
	secs := sidebarNavSections(d)
	if len(secs) != 2 {
		t.Fatalf("want Reports+Queries, got %d %+v", len(secs), secs)
	}
	if secs[0].Title != DefaultSidebarSection || secs[1].Title != "Queries" {
		t.Fatalf("titles: %q %q", secs[0].Title, secs[1].Title)
	}
	if len(secs[1].Pages) != 0 {
		t.Fatalf("queries page not in sidebar links: %v", ids(secs[1].Pages))
	}
	if len(secs[1].Queries) != 2 || secs[1].Queries[0] != "cash" {
		t.Fatalf("Queries: %v", secs[1].Queries)
	}
}

func TestPageRegistryDuplicatePanics(t *testing.T) {
	r := NewPageRegistry()
	r.Register(Page{
		ID: "x", Label: "X", Body: func(d PageData) templ.Component { return templ.NopComponent },
	})
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate")
		}
	}()
	r.Register(Page{
		ID: "x", Label: "X2", Body: func(d PageData) templ.Component { return templ.NopComponent },
	})
}

func TestCustomPagesOnServer(t *testing.T) {
	root := exampleRoot(t)
	p, pdb, _, err := engine.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewPageRegistry()
	// Minimal set: only check, so custom slug 404s and build only expands check.
	reg.Register(Page{
		ID: "check", Label: "Check", Order: 10, Sidebar: true, Build: true,
		Body: func(d PageData) templ.Component { return checkBody(d) },
	})
	s, err := New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	s.Pages = reg
	h := s.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/check", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("check status %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/balances", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("balances should 404 with custom registry, got %d", rr.Code)
	}

	sess := NewSession(root)
	if _, _, err := sess.Project(t.Context()); err != nil {
		t.Fatal(err)
	}
	insts, err := DefaultRegistry(s).Instances(t.Context(), sess)
	if err != nil {
		t.Fatal(err)
	}
	var ledgerReports int
	for _, inst := range insts {
		if inst.Kind != KindPage {
			continue
		}
		// count /l/{name}/{page} but not account/commodity/root
		// personal/check should exist; personal/balances should not
		if inst.Path == "/l/personal/check" {
			ledgerReports++
		}
		if inst.Path == "/l/personal/balances" {
			t.Fatalf("build should not expand balances with custom registry: %v", inst.Path)
		}
	}
	if ledgerReports != 1 {
		t.Fatalf("want 1 personal/check instance, counted paths carefully via balances absence")
	}
}

func ids(pages []Page) []string {
	out := make([]string, len(pages))
	for i, p := range pages {
		out[i] = p.ID
	}
	return out
}
