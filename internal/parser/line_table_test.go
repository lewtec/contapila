package parser

import (
	"testing"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/source"
)

func TestSourceFileLineAtViaParse(t *testing.T) {
	// Line mapping is owned by grammar.LineIndex on source.File.Lines.
	f := source.NewString("t.beancount", "a\nbb\nccc\n")
	if f.Lines.LineAt(5) != 3 {
		t.Fatalf("Lines.LineAt(5)=%d", f.Lines.LineAt(5))
	}
}

func TestParseDirectiveLines(t *testing.T) {
	src := []byte("2020-01-01 open Assets:Cash\n2020-01-02 open Expenses:Food\n")
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("%v", diags)
	}
	if len(dirs) != 2 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	o0, ok0 := dirs[0].(ast.Open)
	o1, ok1 := dirs[1].(ast.Open)
	if !ok0 || !ok1 {
		t.Fatalf("types %T %T", dirs[0], dirs[1])
	}
	if o0.Line != 1 || o1.Line != 2 {
		t.Fatalf("lines %d %d", o0.Line, o1.Line)
	}
	if o0.File != "t.beancount" {
		t.Fatalf("file=%s", o0.File)
	}
}
