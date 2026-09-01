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
		if !bytes.Contains(html, []byte(`id="cmdpalette"`)) {
			t.Fatalf("%s: missing command palette", page)
		}
		if !bytes.Contains(html, []byte(`id="cmdpalette-open"`)) {
			t.Fatalf("%s: missing Jump button", page)
		}
		if !bytes.Contains(html, []byte(`>Jump<`)) {
			t.Fatalf("%s: Jump label missing", page)
		}
		if !bytes.Contains(html, []byte(`data-cmdpalette-js`)) {
			t.Fatalf("%s: missing cmdpalette script assets", page)
		}
		if !bytes.Contains(html, []byte(`data-cmdpalette`)) {
			t.Fatalf("%s: missing data-cmdpalette root", page)
		}
		if !bytes.Contains(html, []byte(`href="/static/logo.png"`)) {
			t.Fatalf("%s: missing favicon", page)
		}
		if !bytes.Contains(html, []byte(`src="/static/logo.png"`)) {
			t.Fatalf("%s: missing brand logo", page)
		}
		if bytes.Contains(html, []byte(`data:image/svg+xml`)) {
			t.Fatalf("%s: old inline favicon still present", page)
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
		if !bytes.Contains(buf.Bytes(), []byte(`id="cmdpalette"`)) {
			t.Fatal("missing command palette")
		}
	}
}
