package parser

import (
	"os"
	"path/filepath"
	"testing"
)

// small multi-directive source: open + balance + transaction
var smallSrc = []byte(`
2020-01-01 open Assets:Cash BRL
2020-01-01 open Expenses:Food BRL
2020-01-01 balance Assets:Cash 0 BRL
2020-01-02 * "Lunch"
  Assets:Cash  -30.00 BRL
  Expenses:Food
`)

func BenchmarkParse_small(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		dirs, diags, err := Parse("bench.beancount", smallSrc)
		if err != nil {
			b.Fatal(err)
		}
		if diags.HasErrors() {
			b.Fatalf("diags: %v", diags)
		}
		if len(dirs) == 0 {
			b.Fatal("no directives")
		}
	}
}

func BenchmarkParse_file(b *testing.B) {
	path := filepath.Join("..", "..", "testdata", "example", "personal", "expenses.beancount")
	src, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		dirs, diags, err := Parse(path, src)
		if err != nil {
			b.Fatal(err)
		}
		if diags.HasErrors() {
			b.Fatalf("diags: %v", diags)
		}
		if len(dirs) == 0 {
			b.Fatal("no directives")
		}
	}
}

func BenchmarkParse_prices(b *testing.B) {
	path := filepath.Join("..", "..", "testdata", "example", "prices.beancount")
	src, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		dirs, diags, err := Parse(path, src)
		if err != nil {
			b.Fatal(err)
		}
		if diags.HasErrors() {
			b.Fatalf("diags: %v", diags)
		}
		if len(dirs) == 0 {
			b.Fatal("no directives")
		}
	}
}
