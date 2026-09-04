package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenLedgerSurfacesDocsFolderDateDiags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "contapila.cue"), []byte("//\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := filepath.Join(root, "personal")
	docDir := filepath.Join(ledger, "docs", "by-account", "Assets", "Cash")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledger, "main.beancount"), []byte(`
option "operating_currency" "BRL"
2020-01-01 open Assets:Cash BRL
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docDir, "20241301_bad.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, pdb, _, err := OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(t.Context(), p, pdb, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if !l.Diags.HasErrors() {
		t.Fatalf("want docs_folder date error on ledger diags: %v", l.Diags)
	}
	found := false
	for _, d := range l.Diags {
		if d.IsError() && strings.Contains(d.Message, "20241301") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing invalid date diag: %v", l.Diags)
	}
}
