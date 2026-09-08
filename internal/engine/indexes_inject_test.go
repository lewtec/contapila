package engine

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lucasew/contapila-go/internal/ast"
)

func TestRootIndexesAutoInjected(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	// internal/engine → repo root → testdata/example
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "example")
	p, pdb, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.StreamJournals) == 0 {
		t.Fatal("expected stream journals from prelude (indexes.beancount)")
	}
	l, err := OpenLedger(p, pdb, "personal")
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for _, d := range l.Dirs {
		if c, ok := d.(ast.Custom); ok && c.Type == "index" {
			n++
		}
	}
	if n < 100 {
		t.Fatalf("expected many index customs from auto-inject, got %d", n)
	}
	if len(l.IndexDB["CDI"]) < 100 {
		t.Fatalf("IndexDB CDI=%d", len(l.IndexDB["CDI"]))
	}
	// acme also gets inject even without interest accounts
	acme, err := OpenLedger(p, pdb, "acme")
	if err != nil {
		t.Fatal(err)
	}
	var n2 int
	for _, d := range acme.Dirs {
		if c, ok := d.(ast.Custom); ok && c.Type == "index" {
			n2++
		}
	}
	if n2 != n {
		t.Fatalf("acme index customs %d want %d", n2, n)
	}
}
