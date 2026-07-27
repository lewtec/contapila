package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectJournalPathRejectsEscape(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"../outside.beancount",
		"..",
		"/etc/passwd",
		"foo/../../etc/passwd",
		"",
	} {
		if _, err := resolveProjectJournalPath(root, rel); err == nil {
			t.Fatalf("resolveProjectJournalPath(%q) accepted escape", rel)
		}
	}
}

func TestResolveProjectJournalPathOK(t *testing.T) {
	root := t.TempDir()
	abs, err := resolveProjectJournalPath(root, "prices.beancount")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "prices.beancount")
	if abs != want {
		t.Fatalf("abs=%q want %q", abs, want)
	}
	// nested relative
	abs, err = resolveProjectJournalPath(root, "data/./indexes.beancount")
	if err != nil {
		t.Fatal(err)
	}
	if abs != filepath.Join(root, "data/indexes.beancount") {
		t.Fatalf("nested abs=%q", abs)
	}
}

func TestOpenProjectRejectsEscapingJournal(t *testing.T) {
	root := t.TempDir()
	// minimal project: marker + one ledger
	if err := os.WriteFile(filepath.Join(root, "contapila.cue"), []byte(`
project_journals: [
  {path: "../secret.beancount", role: "stream", missing: "ignore"},
]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := filepath.Join(root, "personal")
	if err := os.MkdirAll(ledger, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledger, "main.beancount"), []byte("2020-01-01 open Assets:Cash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := OpenProject(root)
	if err == nil {
		t.Fatal("expected error for escaping project_journals path")
	}
	if !errors.Is(err, ErrJournalPathEscapes) && !errors.Is(err, ErrJournalPathAbsolute) {
		t.Fatalf("err=%v want ErrJournalPathEscapes or ErrJournalPathAbsolute", err)
	}
}
