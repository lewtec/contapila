package web

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/prices"
)

func TestChartPriceJSONExample(t *testing.T) {
	_, f, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(f), "..", "..", "testdata", "example", "prices.beancount")
	db, _, err := prices.LoadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	js, quote, err := chartPriceJSON(db, "USD", "BRL", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if js == "" {
		t.Fatal("empty chart json")
	}
	if quote != "BRL" {
		t.Fatalf("quote=%s", quote)
	}
}

func TestChartPriceJSONYear2026Empty(t *testing.T) {
	_, f, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(f), "..", "..", "testdata", "example", "prices.beancount")
	db, _, err := prices.LoadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	// If UI defaults to "year" = 2026, prices from 2023-2025 vanish.
	js, _, err := chartPriceJSON(db, "USD", "BRL",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if js != "" {
		t.Log("unexpected data in 2026")
	} else {
		t.Log("confirmed: 2026 filter yields no chart")
	}
}
