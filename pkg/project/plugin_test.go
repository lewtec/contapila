package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/config"
)

func TestOpenProjectJournalPluginEnables(t *testing.T) {
	config.RegisterKnownPlugin("web/accounts")
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
plugin "web/accounts"
2020-01-01 open Assets:Cash BRL
`
	if err := os.WriteFile(filepath.Join(ledgerDir, "main.beancount"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := OpenProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := config.PluginEnabled(p.Config.Value, "web/accounts")
	if err != nil || !ok {
		t.Fatalf("want web/accounts enabled from plugin directive: ok=%v err=%v", ok, err)
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
plugin "web/does-not-exist"
2020-01-01 open Assets:Cash BRL
`), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := OpenProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := config.PluginEnabled(p.Config.Value, "web/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("unknown plugin must not enable in CUE")
	}
	if !p.Diags.HasWarnings() {
		t.Fatal("want project-level warn diag for unknown plugin")
	}
	found := false
	for _, d := range p.Diags {
		if d.IsWarn() && strings.Contains(d.Message, "web/does-not-exist") {
			found = true
			if d.Line < 1 {
				t.Fatalf("want line number on diag: %+v", d)
			}
		}
	}
	if !found {
		t.Fatalf("diag message missing: %v", p.Diags)
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
	p, err := OpenProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := config.PluginEnabled(p.Config.Value, "web/accounts")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("want web/accounts off without plugin directive")
	}
}
