package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/contapila-go/internal/config"
)

func TestOpenProjectJournalPluginDoesNotMutateCUE(t *testing.T) {
	config.RegisterKnownPlugin("web_accounts")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "contapila.cue"), []byte("// empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledgerDir := filepath.Join(dir, "personal")
	if err := os.Mkdir(ledgerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	main := `
option "operating_currency" "BRL"
plugin "web_accounts"
2020-01-01 open Assets:Cash BRL
`
	if err := os.WriteFile(filepath.Join(ledgerDir, "main.beancount"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := OpenProject(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := config.PluginEnabled(p.Config.Value, "web_accounts")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("journal plugin must not mutate project CUE; enable is per-ledger at open")
	}
}

func TestOpenProjectUnknownPluginSkipped(t *testing.T) {
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
	p, err := OpenProject(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := config.PluginEnabled(p.Config.Value, "web_does_not_exist")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("unknown plugin must not enable in CUE")
	}
}

func TestOpenProjectNoPluginStaysOff(t *testing.T) {
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
2020-01-01 open Assets:Cash BRL
`), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := OpenProject(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := config.PluginEnabled(p.Config.Value, "web_accounts")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("want web_accounts off without plugin directive")
	}
}
