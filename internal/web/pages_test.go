package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a-h/templ"

	"github.com/lucasew/contapila-go/internal/engine"
)

func TestDefaultPagesSidebarAndBuild(t *testing.T) {
	resetContributedPagesForTest()
	t.Cleanup(resetContributedPagesForTest)

	r := DefaultPages()
	side := r.Sidebar()
	if len(side) != 7 {
		t.Fatalf("sidebar len %d want 7: %v", len(side), ids(side))
	}
	if side[0].ID != "check" || side[len(side)-1].ID != "prices" {
		t.Fatalf("sidebar order: %v", ids(side))
	}
	build := r.BuildIDs()
	if len(build) != 7 || build[0] != "check" || build[len(build)-1] != "prices" {
		t.Fatalf("build ids: %v", build)
	}
	// With no contributions, ReportPages matches builtins.
	rp := ReportPages()
	if len(rp) != len(build) {
		t.Fatalf("ReportPages %v vs BuildIDs %v", rp, build)
	}
	for i := range rp {
		if rp[i] != build[i] {
			t.Fatalf("ReportPages[%d]=%q BuildIDs[%d]=%q", i, rp[i], i, build[i])
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
	p, pdb, _, err := engine.OpenProject(root)
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
	if _, _, err := sess.Project(); err != nil {
		t.Fatal(err)
	}
	insts, err := DefaultRegistry(s).Instances(sess)
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
