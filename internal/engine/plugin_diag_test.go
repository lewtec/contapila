package engine

import (
	"context"
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
plugin "web_does_not_exist"
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
		if d.IsWarn() && strings.Contains(d.Message, "web_does_not_exist") {
			found = true
		}
	}
	if !found {
		t.Fatalf("check diags missing unknown plugin: %v", l.Diags)
	}
}

func TestJournalPluginEnablesCheckClosing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "contapila.cue"), []byte("// empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ld := filepath.Join(dir, "personal")
	if err := os.Mkdir(ld, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ld, "main.beancount"), []byte(`
option "operating_currency" "BRL"
plugin "check_closing"
2020-01-01 open Assets:Cash BRL
2020-01-01 open Equity:Opening BRL
2020-01-02 * "seed"
  Assets:Cash   10.00 BRL
  Equity:Opening
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
	if !l.PluginEnabled("check_closing", false) {
		t.Fatal("journal plugin check_closing should enable the module")
	}
	if l.PluginEnabled("web_accounts", false) {
		t.Fatal("unmentioned opt-in module should stay off")
	}
}

func TestOpenLedgerCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p, pdb, _, err := OpenProject(filepath.Join("..", "..", "testdata", "example"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenLedgerFS(ctx, nil, p, pdb, "personal"); err == nil {
		t.Fatal("expected canceled context error")
	}
}
