package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/parser"
)

func TestDecodeCustomIndex(t *testing.T) {
	line := `{"type":"custom","date":"2025-04-03","id":"cdi:2025-04-03","custom_type":"index","values":[{"text":"CDI"},{"number":"0.000451"}]}` + "\n"
	dirs, err := DecodeJSONL(strings.NewReader(line), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("n=%d", len(dirs))
	}
	c, ok := dirs[0].(ast.Custom)
	if !ok || c.Type != "index" {
		t.Fatalf("%T %+v", dirs[0], dirs[0])
	}
	if c.Metadata[ast.IngestIDMetaKey] != "cdi:2025-04-03" {
		t.Fatalf("meta=%v", c.Metadata)
	}
	if len(c.Values) != 2 || c.Values[0].Text != "CDI" || c.Values[1].Number == nil {
		t.Fatalf("values=%+v", c.Values)
	}
}

func TestApplyAppendAndReplace(t *testing.T) {
	base := `; hand written
2025-01-01 open Assets:Cash BRL

2025-04-03 custom "index" "CDI" 0.0001
  ingest_id: "cdi:2025-04-03"
`
	// replace existing + append new without id
	in := strings.Join([]string{
		`{"type":"custom","date":"2025-04-03","id":"cdi:2025-04-03","custom_type":"index","values":[{"text":"CDI"},{"number":"0.000451"}]}`,
		`{"type":"custom","date":"2025-04-04","custom_type":"index","values":[{"text":"CDI"},{"number":"0.0004"}]}`,
	}, "\n") + "\n"
	dirs, err := DecodeJSONL(strings.NewReader(in), nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Apply(base, "t.beancount", dirs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "; hand written") {
		t.Fatalf("lost hand content:\n%s", out)
	}
	if !strings.Contains(out, "0.000451") {
		t.Fatalf("missing replaced rate:\n%s", out)
	}
	if strings.Count(out, `ingest_id: "cdi:2025-04-03"`) != 1 {
		t.Fatalf("id count:\n%s", out)
	}
	// append without id should not have ingest_id
	if !strings.Contains(out, "2025-04-04 custom") {
		t.Fatalf("missing append:\n%s", out)
	}
	// re-parse
	parsed, diags, err := parser.Parse("t.beancount", []byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("%v", diags)
	}
	var customs int
	for _, d := range parsed {
		if _, ok := d.(ast.Custom); ok {
			customs++
		}
	}
	if customs != 2 {
		t.Fatalf("customs=%d out=\n%s", customs, out)
	}
}

func TestApplyParseErrorReadable(t *testing.T) {
	// Garbage that tree-sitter cannot turn into directives → error diagnostics.
	bad := "@@@ not beancount @@@\n"
	_, err := Apply(bad, "bad.beancount", nil)
	if err == nil {
		t.Fatal("expected parse error")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "parse bad.beancount: ") {
		t.Fatalf("prefix: %q", msg)
	}
	// Human-readable Diagnostic.String form, not fmt %v of the slice/structs.
	if strings.Contains(msg, "Severity:") || strings.Contains(msg, "[{") {
		t.Fatalf("raw struct dump: %q", msg)
	}
	if !strings.Contains(msg, "error:") {
		t.Fatalf("expected severity label: %q", msg)
	}
}

func TestApplyCreateFromEmpty(t *testing.T) {
	line := `{"type":"price","date":"2025-01-01","id":"px:usd","currency":"USD","amount":{"number":"5.2","commodity":"BRL"}}` + "\n"
	dirs, err := DecodeJSONL(strings.NewReader(line), nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Apply("", "new.beancount", dirs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "price USD") || !strings.Contains(out, `ingest_id: "px:usd"`) {
		t.Fatalf("%s", out)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "out.beancount")
	if err := WriteFileAtomic(path, []byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello\n" {
		t.Fatalf("%q", b)
	}
}

func TestDuplicateIDLastWins(t *testing.T) {
	in := strings.Join([]string{
		`{"type":"custom","date":"2025-04-03","id":"x","custom_type":"index","values":[{"text":"CDI"},{"number":"0.1"}]}`,
		`{"type":"custom","date":"2025-04-03","id":"x","custom_type":"index","values":[{"text":"CDI"},{"number":"0.2"}]}`,
	}, "\n") + "\n"
	var warn strings.Builder
	dirs, err := DecodeJSONL(strings.NewReader(in), &warn)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("n=%d", len(dirs))
	}
	c := dirs[0].(ast.Custom)
	if c.Values[1].Number.FloatString(1) != "0.2" {
		t.Fatalf("got %s", c.Values[1].Number.FloatString(4))
	}
	if !strings.Contains(warn.String(), "duplicate") {
		t.Fatalf("warn=%q", warn.String())
	}
}

func TestFormatRoundTripCustomMeta(t *testing.T) {
	src := `2025-04-03 custom "index" "CDI" 0.000451
  ingest_id: "cdi:2025-04-03"
`
	dirs, diags, err := parser.Parse("t", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	c := dirs[0].(ast.Custom)
	if c.Metadata["ingest_id"] != "cdi:2025-04-03" {
		t.Fatalf("meta=%v", c.Metadata)
	}
	if c.StartByte == 0 && c.EndByte == 0 {
		// empty file offsets can be 0 for start - ok if end > start
	}
	if c.EndByte <= c.StartByte {
		t.Fatalf("span %d-%d", c.StartByte, c.EndByte)
	}
}
