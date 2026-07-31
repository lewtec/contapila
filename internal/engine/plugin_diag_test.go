package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenLedgerMergesProjectPluginDiags(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "contapila.cue"), []byte("// empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledgerDir := filepath.Join(dir, "personal")
	if err := os.Mkdir(ledgerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledgerDir, "main.beancount"), []byte(`
option "operating_currency" "BRL"
plugin "web/does-not-exist"
2020-01-01 open Assets:Cash BRL
`), 0o644); err != nil {
		t.Fatal(err)
	}
	p, pdb, _, err := OpenProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(p, pdb, "personal")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range l.Diags {
		if d.IsWarn() && strings.Contains(d.Message, "web/does-not-exist") {
			found = true
		}
	}
	if !found {
		t.Fatalf("check diags missing unknown plugin: %v", l.Diags)
	}
}
