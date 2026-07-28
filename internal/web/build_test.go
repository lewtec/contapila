package web

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildExample(t *testing.T) {
	root := exampleRoot(t)
	out := t.TempDir()
	if err := Build(root, out); err != nil {
		t.Fatal(err)
	}

	mustExist := []string{
		"index.html",
		"static/app.css",
		"static/charts.js",
		"l/personal/index.html", // ledger root (live redirect → check)
		"l/acme/index.html",
		"l/personal/check/index.html",
		"l/personal/pnl/index.html",
		"l/personal/networth/index.html",
		"l/acme/check/index.html",
	}
	for _, rel := range mustExist {
		path := filepath.Join(out, rel)
		st, err := os.Stat(path)
		if err != nil {
			t.Errorf("missing %s: %v", rel, err)
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
	if !strings.Contains(string(index), `href="/l/personal/check"`) {
		t.Fatalf("index missing personal check link: %s", truncate(string(index), 200))
	}

	// Ledger root is check content (redirect followed at build time).
	rootHTML, err := os.ReadFile(filepath.Join(out, "l", "acme", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	checkHTML, err := os.ReadFile(filepath.Join(out, "l", "acme", "check", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(rootHTML) != string(checkHTML) {
		t.Fatalf("l/acme/index.html should match check page body (got %d vs %d bytes)", len(rootHTML), len(checkHTML))
	}

	// At least one account page and commodity page under personal.
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
		if strings.Contains(rel, "/account/") && strings.HasSuffix(rel, "/index.html") {
			acct++
		}
		if strings.Contains(rel, "/commodity/") && strings.HasSuffix(rel, "/index.html") {
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
	err := Build(exampleRoot(t), "")
	if !errors.Is(err, ErrBuildOutRequired) {
		t.Fatalf("got %v want %v", err, ErrBuildOutRequired)
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
