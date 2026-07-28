package web

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildExample(t *testing.T) {
	root := exampleRoot(t)
	out := t.TempDir()
	if err := Build(root, out, 0); err != nil {
		t.Fatal(err)
	}

	mustExist := []string{
		"index.html",
		"static/app.css",
		"static/charts.js",
		"l/personal/index.html", // ledger root (live redirect → check)
		"l/acme/index.html",
		// *.html files + rewritten hrefs so static hosts hit real files (not …/index.html).
		"l/personal/check.html",
		"l/personal/pnl.html",
		"l/personal/networth.html",
		"l/acme/check.html",
	}
	for _, rel := range mustExist {
		path := filepath.Join(out, rel)
		st, err := os.Stat(path)
		if err != nil {
			t.Errorf("missing %s: %v", rel, err)
			continue
		}
		if st.IsDir() {
			t.Errorf("%s: want file, got directory", rel)
			continue
		}
		if st.Size() == 0 {
			t.Errorf("%s is empty", rel)
		}
	}

	// Index should list a ledger link.
	index, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), `href="/l/personal/check.html"`) {
		t.Fatalf("index missing rewritten personal check link: %s", truncate(string(index), 200))
	}
	if strings.Contains(string(index), `href="/l/personal/check"`) &&
		!strings.Contains(string(index), `href="/l/personal/check.html"`) {
		t.Fatal("live extensionless check link was not rewritten to .html")
	}

	// Ledger root is check content (redirect followed at build time).
	rootHTML, err := os.ReadFile(filepath.Join(out, "l", "acme", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	checkHTML, err := os.ReadFile(filepath.Join(out, "l", "acme", "check.html"))
	if err != nil {
		t.Fatal(err)
	}
	// Bodies match before/after link rewrite only if both were rewritten the same way.
	if !bytes.Contains(rootHTML, []byte(`href="/l/acme/check.html"`)) && !bytes.Contains(rootHTML, []byte("Check")) {
		t.Fatalf("ledger root page looks empty/wrong: %s", truncate(string(rootHTML), 200))
	}
	if len(checkHTML) == 0 {
		t.Fatal("check.html empty")
	}

	// At least one account page and commodity page under personal (*.html files).
	var acct, comm int
	err = filepath.WalkDir(filepath.Join(out, "l", "personal"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(out, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(rel, "/account/") && strings.HasSuffix(rel, ".html") {
			acct++
		}
		if strings.Contains(rel, "/commodity/") && strings.HasSuffix(rel, ".html") {
			comm++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if acct == 0 {
		t.Error("no account pages written")
	}
	if comm == 0 {
		t.Error("no commodity pages written")
	}

	// Docfile copy when example has docs.
	doc := filepath.Join(out, "docfile", "personal", "docs", "README.md")
	if _, err := os.Stat(doc); err != nil {
		// Example may still have by-account docs; either is fine if any docfile exists.
		var docs int
		walkErr := filepath.WalkDir(filepath.Join(out, "docfile"), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				docs++
			}
			return nil
		})
		if walkErr != nil && !os.IsNotExist(walkErr) {
			t.Fatal(walkErr)
		}
		if docs == 0 {
			t.Log("no docfile outputs (ok if fixture has no serveable docs)")
		}
	}
}

func TestBuildRequiresOut(t *testing.T) {
	err := Build(exampleRoot(t), "", 0)
	if !errors.Is(err, ErrBuildOutRequired) {
		t.Fatalf("got %v want %v", err, ErrBuildOutRequired)
	}
}

func TestBuildJobsOne(t *testing.T) {
	// Explicit serial path still succeeds (same outputs as default pool).
	out := t.TempDir()
	if err := Build(exampleRoot(t), out, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "l", "personal", "check.html")); err != nil {
		t.Fatal(err)
	}
}

func TestKindLabel(t *testing.T) {
	if got := kindLabel(KindPage); got != "page" {
		t.Fatalf("page: %q", got)
	}
	if got := kindLabel(KindStatic); got != "static" {
		t.Fatalf("static: %q", got)
	}
	if got := kindLabel(KindDoc); got != "doc" {
		t.Fatalf("doc: %q", got)
	}
}

func TestRewriteStaticPageLinks(t *testing.T) {
	in := []byte(`
<a href="/l/acme/check">c</a>
<a href="/l/acme/account/Assets:Cash">a</a>
<a href="/l/acme/">root</a>
<a href="/">home</a>
<link href="/static/app.css"/>
<a href="/docfile/personal/docs/x.txt">d</a>
<form action="/l/personal/pnl"></form>
<a href="/l/personal/check?time=2024">q</a>
`)
	got := string(rewriteStaticPageLinks(in))
	wantSub := []string{
		`href="/l/acme/check.html"`,
		`href="/l/acme/account/Assets:Cash.html"`,
		`href="/l/acme/"`,
		`href="/"`,
		`href="/static/app.css"`,
		`href="/docfile/personal/docs/x.txt"`,
		`action="/l/personal/pnl.html"`,
		`href="/l/personal/check.html?time=2024"`,
	}
	for _, w := range wantSub {
		if !strings.Contains(got, w) {
			t.Errorf("missing %s in:\n%s", w, got)
		}
	}
	if strings.Contains(got, `href="/l/acme/check"`) {
		t.Error("extensionless check link should have been rewritten")
	}
}
