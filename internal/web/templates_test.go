package web

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/a-h/templ"

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
	_ = s
	// Render a few pages that use partials
	for _, page := range []string{"balances", "pnl", "networth", "journal", "documents"} {
		data := pageData{
			Title: page, Page: page, LedgerName: "personal",
			Ledgers: []string{"personal"}, ProjectRoot: p.Root,
			OpCurrency: "BRL", PeriodLabel: "2024", Time: "2024",
		}
		var buf bytes.Buffer
		if err := LedgerPage(data).Render(t.Context(), &buf); err != nil {
			t.Fatalf("%s: %v", page, err)
		}
		if buf.Len() < 100 {
			t.Fatalf("%s: short render %d", page, buf.Len())
		}
		html := buf.Bytes()
		if !bytes.Contains(html, []byte(`id="cmd-palette"`)) {
			t.Fatalf("%s: missing command palette", page)
		}
		if !bytes.Contains(html, []byte(`id="cmd-palette-open"`)) {
			t.Fatalf("%s: missing Jump button", page)
		}
		if !bytes.Contains(html, []byte(`>Jump<`)) {
			t.Fatalf("%s: Jump label missing", page)
		}
		if !bytes.Contains(html, []byte(`/static/command.js`)) {
			t.Fatalf("%s: missing command.js", page)
		}
		// daisyUI .modal fights native <dialog> open visibility — must not use it.
		if bytes.Contains(html, []byte(`id="cmd-palette" class="modal`)) {
			t.Fatalf("%s: palette must not use daisyUI modal class", page)
		}
	}
	// account + commodity
	for _, page := range []templ.Component{
		AccountPage(pageData{
			Title: "x", Page: "account", LedgerName: "personal",
			Ledgers: []string{"personal"}, AccountName: "Assets:Cash",
		}),
		CommodityPage(pageData{
			Title: "x", Page: "commodity", LedgerName: "personal",
			Ledgers: []string{"personal"}, CommodityName: "BRL",
		}),
		IndexPage(pageData{
			Title: "Ledgers", Page: "home", Ledgers: []string{"personal"}, ProjectRoot: p.Root,
		}),
	} {
		var buf bytes.Buffer
		if err := page.Render(t.Context(), &buf); err != nil {
			t.Fatalf("render: %v", err)
		}
		if buf.Len() < 100 {
			t.Fatalf("short render %d", buf.Len())
		}
		if !bytes.Contains(buf.Bytes(), []byte(`id="cmd-palette"`)) {
			t.Fatal("missing command palette")
		}
	}
}
