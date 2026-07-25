package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/pkg/project"
)

func TestCanonicalPathResolvesSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "indexes.beancount")
	if err := os.WriteFile(real, []byte("2020-01-01 custom \"index\" \"CDI\" 0.01\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "indexes-link.beancount")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if canonicalPath(real) != canonicalPath(link) {
		t.Fatalf("canonical paths differ: %q vs %q", canonicalPath(real), canonicalPath(link))
	}
}

func TestInjectSkipsStreamJournalAlreadyIncludedViaSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "indexes.beancount")
	if err := os.WriteFile(real, []byte("2020-01-01 custom \"index\" \"CDI\" 0.01\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "via-link.beancount")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	stream := []ast.Directive{
		ast.Custom{Meta: ast.Meta{Date: day, File: link}, Type: "index"},
	}
	p := &project.Project{
		StreamJournals: []project.StreamJournal{{Path: real, RelPath: "indexes.beancount"}},
	}
	out, diags := injectProjectStreamJournals(nil, p, stream)
	if len(diags) != 0 {
		t.Fatalf("diags=%v", diags)
	}
	if len(out) != len(stream) {
		t.Fatalf("expected no inject (dedupe via realpath), got %d dirs (was %d)", len(out), len(stream))
	}
}
