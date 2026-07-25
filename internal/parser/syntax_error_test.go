package parser

import (
	"strings"
	"testing"
)

func TestParseSyntaxErrorNearSnippet(t *testing.T) {
	// ERROR nodes surface as diags via clip(); previously 0% on clip and no
	// dedicated syntax-error tests.
	dirs, diags, err := Parse("bad.beancount", []byte("@@@ garbage\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 0 {
		t.Fatalf("dirs=%d want 0", len(dirs))
	}
	if len(diags) == 0 {
		t.Fatal("expected syntax error diagnostic")
	}
	msg := diags[0].Message
	if !strings.Contains(msg, "syntax error near") {
		t.Fatalf("message=%q", msg)
	}
	if !strings.Contains(msg, "@@@ garbage") {
		t.Fatalf("expected clipped source in message, got %q", msg)
	}
}

func TestParseSyntaxErrorClipsLongSnippet(t *testing.T) {
	// clip max is 40 runes of flattened text + ellipsis.
	long := strings.Repeat("x", 60)
	src := []byte(long + "\n")
	_, diags, err := Parse("long.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) == 0 {
		t.Fatal("expected syntax error")
	}
	msg := diags[0].Message
	// Quoted snippet should be truncated with ellipsis, not the full 60 x's.
	if strings.Count(msg, "x") >= 60 {
		t.Fatalf("snippet not clipped: %q", msg)
	}
	if !strings.Contains(msg, "…") {
		t.Fatalf("expected ellipsis in clipped snippet: %q", msg)
	}
}

func TestClipHelper(t *testing.T) {
	if got := clip("short", 40); got != "short" {
		t.Fatalf("short: %q", got)
	}
	if got := clip("a\nb", 40); got != "a\\nb" {
		t.Fatalf("newline: %q", got)
	}
	got := clip(strings.Repeat("z", 50), 40)
	if got != strings.Repeat("z", 40)+"…" {
		t.Fatalf("trunc: %q", got)
	}
}

func TestParseIncompleteOpenIsSyntaxError(t *testing.T) {
	_, diags, err := Parse("t.beancount", []byte("2024-01-01 open\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) == 0 {
		t.Fatal("expected diags")
	}
	if !strings.Contains(diags[0].Message, "syntax error near") {
		t.Fatalf("message=%q", diags[0].Message)
	}
}
