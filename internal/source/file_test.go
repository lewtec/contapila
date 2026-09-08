package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStringBuildsLines(t *testing.T) {
	f := NewString("t.beancount", "a\nbb\nccc\n")
	if f.Path != "t.beancount" {
		t.Fatalf("path=%s", f.Path)
	}
	if f.Text != "a\nbb\nccc\n" {
		t.Fatalf("text=%q", f.Text)
	}
	if f.Lines == nil {
		t.Fatal("Lines is nil")
	}
	// Spot-check wiring to grammar.LineIndex (full table lives upstream).
	if got := f.Lines.LineAt(5); got != 3 {
		t.Fatalf("Lines.LineAt(5)=%d; want 3", got)
	}
	if got := f.Lines.LineAtU32(2); got != 2 {
		t.Fatalf("Lines.LineAtU32(2)=%d; want 2", got)
	}
}

func TestSlice(t *testing.T) {
	f := NewString("t", "hello")

	tests := []struct {
		name       string
		f          *File
		start, end int
		want       string
	}{
		{name: "middle", f: f, start: 1, end: 4, want: "ell"},
		{name: "full", f: f, start: 0, end: 5, want: "hello"},
		{name: "first byte", f: f, start: 0, end: 1, want: "h"},
		{name: "last byte", f: f, start: 4, end: 5, want: "o"},
		{name: "empty range in bounds", f: f, start: 2, end: 2, want: ""},
		{name: "empty at end", f: f, start: 5, end: 5, want: ""},
		{name: "negative start", f: f, start: -1, end: 2, want: ""},
		{name: "end past len", f: f, start: 0, end: 99, want: ""},
		{name: "start past end", f: f, start: 3, end: 1, want: ""},
		{name: "start past len", f: f, start: 6, end: 7, want: ""},
		{name: "both negative", f: f, start: -2, end: -1, want: ""},
		{name: "nil receiver", f: nil, start: 0, end: 1, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.f.Slice(tt.start, tt.end); got != tt.want {
				t.Errorf("Slice(%d, %d) = %q; want %q", tt.start, tt.end, got, tt.want)
			}
		})
	}
}

func TestNewReadsDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.beancount")
	if err := os.WriteFile(path, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Path != path {
		t.Fatalf("path=%s", f.Path)
	}
	if f.Lines.LineAt(6) != 2 {
		t.Fatalf("Lines.LineAt(6)=%d", f.Lines.LineAt(6))
	}
	if f.Text != "line1\nline2\n" {
		t.Fatalf("text=%q", f.Text)
	}
}

func TestNewMissingFile(t *testing.T) {
	_, err := New(filepath.Join(t.TempDir(), "missing.beancount"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
