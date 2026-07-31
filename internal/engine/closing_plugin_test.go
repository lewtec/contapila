package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/config"
	_ "github.com/lucasew/contapila-go/internal/plugins/checkclosing"
)

func TestCheckClosingPluginGatesAutoclose(t *testing.T) {
	// checkclosing init registers the module + known plugin name.

	writeProj := func(t *testing.T, main string) string {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "contapila.cue"), []byte("// empty\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ld := filepath.Join(dir, "personal")
		if err := os.Mkdir(ld, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ld, "main.beancount"), []byte(main), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	const body = `
2020-01-01 open Assets:Cash BRL
2020-01-01 open Equity:Opening BRL
2020-01-02 * "seed"
  Assets:Cash   10.00 BRL
  Equity:Opening
2020-01-10 * "close cash"
  Assets:Cash  -10.00 BRL
    closing: TRUE
  Equity:Opening
`

	t.Run("disabled skips autoclose and warns", func(t *testing.T) {
		dir := writeProj(t, "option \"operating_currency\" \"BRL\"\n"+body)
		p, pdb, _, err := OpenProject(dir)
		if err != nil {
			t.Fatal(err)
		}
		l, err := OpenLedger(p, pdb, "personal")
		if err != nil {
			t.Fatal(err)
		}
		if countCloses(l.Dirs) != 0 {
			t.Fatalf("expected no synthetic close when plugin off, dirs=%v", l.Dirs)
		}
		foundWarn := false
		for _, dg := range l.Diags {
			if dg.IsWarn() && strings.Contains(dg.Message, "check_closing") {
				foundWarn = true
			}
		}
		if !foundWarn {
			t.Fatalf("want warn about plugin: %v", l.Diags)
		}
	})

	t.Run("enabled expands close", func(t *testing.T) {
		dir := writeProj(t, `
option "operating_currency" "BRL"
plugin "check_closing"
`+body)
		p, pdb, _, err := OpenProject(dir)
		if err != nil {
			t.Fatal(err)
		}
		on, err := config.PluginEnabled(p.Config.Value, "check_closing")
		if err != nil || !on {
			t.Fatalf("plugin should be on: %v %v", on, err)
		}
		l, err := OpenLedger(p, pdb, "personal")
		if err != nil {
			t.Fatal(err)
		}
		if countCloses(l.Dirs) == 0 {
			t.Fatal("expected synthetic close when plugin on")
		}
	})
}

func countCloses(dirs []ast.Directive) int {
	n := 0
	for _, d := range dirs {
		if _, ok := d.(ast.Close); ok {
			n++
		}
	}
	return n
}
