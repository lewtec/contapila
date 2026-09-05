package web

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/lucasew/contapila-go/internal/engine"
)

func TestTemplatesParseAndRenderShell(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "example")
	p, pdb, _, err := engine.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	// Render a few pages that use partials
	for _, page := range []string{"balances", "pnl", "networth", "journal", "documents"} {
		data := pageData{
			Title: page, Page: page, LedgerName: "personal",
			Ledgers: []string{"personal"}, ProjectRoot: p.Root,
			OpCurrency: "BRL", PeriodLabel: "2024", Time: "2024",
		}
		var buf bytes.Buffer
		if err := s.Tmpl.ExecuteTemplate(&buf, "ledger.html", data); err != nil {
			t.Fatalf("%s: %v", page, err)
		}
		if buf.Len() < 100 {
			t.Fatalf("%s: short render %d", page, buf.Len())
		}
	}
	// account + commodity
	for _, name := range []string{"account.html", "commodity.html"} {
		data := pageData{
			Title: "x", Page: "account", LedgerName: "personal",
			Ledgers: []string{"personal"}, AccountName: "Assets:Cash",
			CommodityName: "BRL",
		}
		var buf bytes.Buffer
		if err := s.Tmpl.ExecuteTemplate(&buf, name, data); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
