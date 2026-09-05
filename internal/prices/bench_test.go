package prices

import (
	"path/filepath"
	"testing"
	"time"
)

func examplePricesPath() string {
	return filepath.Join("..", "..", "testdata", "example", "prices.beancount")
}

func mustLoadExample(b *testing.B) *DB {
	b.Helper()
	db, diags, err := LoadFile(examplePricesPath())
	if err != nil {
		b.Fatal(err)
	}
	if diags.HasErrors() {
		b.Fatalf("diags: %v", diags)
	}
	return db
}

// BenchmarkLoadFile_example times parsing+loading the dogfood prices.beancount
// fixture (~600 lines, multi-commodity).
func BenchmarkLoadFile_example(b *testing.B) {
	path := examplePricesPath()
	// Sanity once so fixture failures fail fast outside the loop.
	if _, diags, err := LoadFile(path); err != nil {
		b.Fatal(err)
	} else if diags.HasErrors() {
		b.Fatalf("diags: %v", diags)
	}

	b.ReportAllocs()
	for b.Loop() {
		db, diags, err := LoadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		if diags.HasErrors() {
			b.Fatalf("diags: %v", diags)
		}
		if len(db.Pairs()) == 0 {
			b.Fatal("expected pairs")
		}
	}
}

// BenchmarkRate_direct times a direct base→quote lookup (USD→BRL) after load.
func BenchmarkRate_direct(b *testing.B) {
	db := mustLoadExample(b)
	asOf := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	if _, _, ok := db.Rate("USD", "BRL", asOf); !ok {
		b.Fatal("expected USD→BRL rate")
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, ok := db.Rate("USD", "BRL", asOf); !ok {
			b.Fatal("expected USD→BRL rate")
		}
	}
}

// BenchmarkRate_inverse times quote→base via inv(base→quote) (BRL→USD).
func BenchmarkRate_inverse(b *testing.B) {
	db := mustLoadExample(b)
	asOf := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	if _, _, ok := db.Rate("BRL", "USD", asOf); !ok {
		b.Fatal("expected BRL→USD inverse rate")
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, ok := db.Rate("BRL", "USD", asOf); !ok {
			b.Fatal("expected BRL→USD inverse rate")
		}
	}
}

// BenchmarkRate_cross times a one-hop conversion (SPDW→USD→BRL).
func BenchmarkRate_cross(b *testing.B) {
	db := mustLoadExample(b)
	asOf := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	if _, _, ok := db.Rate("SPDW", "BRL", asOf); !ok {
		b.Fatal("expected SPDW→BRL cross rate")
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, ok := db.Rate("SPDW", "BRL", asOf); !ok {
			b.Fatal("expected SPDW→BRL cross rate")
		}
	}
}
