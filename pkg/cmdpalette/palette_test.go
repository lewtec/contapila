package cmdpalette

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewNormalizesAndDropsBad(t *testing.T) {
	p := New([]Item{
		{Label: "A", Href: "/a", Group: "G"},
		{Label: "skip"}, // no href
		ItemOf("Q", "All", "/q", "list"),
	})
	if len(p.Items) != 2 {
		t.Fatalf("items %d: %+v", len(p.Items), p.Items)
	}
	if p.Items[0].Keywords == "" {
		t.Fatal("keywords filled")
	}
	if !strings.Contains(p.Items[1].Keywords, "list") {
		t.Fatalf("keywords %q", p.Items[1].Keywords)
	}
}

func TestKeywordsColon(t *testing.T) {
	k := Keywords("account", "Assets:Cash")
	if !strings.Contains(k, "cash") {
		t.Fatalf("%q", k)
	}
}

func TestButtonAndModalRender(t *testing.T) {
	p := New([]Item{ItemOf("Reports", "Check", "/l/x/check")}).
		WithID("jump").
		WithTitle("Jump").
		WithButtonClass("btn btn-xs")
	var buf bytes.Buffer
	if err := p.Button().Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{`id="jump-open"`, `aria-controls="jump"`, "Jump", "btn btn-xs"} {
		if !strings.Contains(html, want) {
			t.Fatalf("button missing %q in %s", want, html)
		}
	}
	buf.Reset()
	if err := p.Modal().Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html = buf.String()
	for _, want := range []string{
		`id="jump"`,
		`data-cmdpalette`,
		`id="jump-list"`,
		`data-cmd-item`,
		"/l/x/check",
		"Check",
		"Reports",
		"cmdpalette-list", // single-column class, not daisy menu
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("modal missing %q", want)
		}
	}
	if strings.Contains(html, `class="menu`) {
		t.Fatal("must not use daisyUI menu")
	}
}

func TestAssetsUnescaped(t *testing.T) {
	var buf bytes.Buffer
	if err := Assets().Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "cmdpalette-dialog") {
		t.Fatal("css missing")
	}
	if !strings.Contains(s, "__cmdpaletteBound") {
		t.Fatal("js missing")
	}
	if strings.Contains(s, "cmdpalette-dialog&#") {
		t.Fatal("css was html-escaped")
	}
}
