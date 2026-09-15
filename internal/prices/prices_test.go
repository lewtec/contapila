package prices

import (
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
)

func TestLastWriteWinsSameDay(t *testing.T) {
	d := time.Date(2023, 7, 15, 0, 0, 0, 0, time.UTC)
	db := NewDB()
	db.AddPrice(ast.Price{
		Meta:     ast.Meta{Date: d},
		Currency: "B3_PETR4",
		Amount:   ast.Amount{Number: big.NewRat(31, 1), Commodity: "BRL"},
		Metadata: ast.Metadata{"source": "first"},
	})
	db.AddPrice(ast.Price{
		Meta:     ast.Meta{Date: d},
		Currency: "B3_PETR4",
		Amount:   ast.Amount{Number: rat("31.50"), Commodity: "BRL"},
		Metadata: ast.Metadata{"source": "second"},
	})
	series := db.SeriesForBase("B3_PETR4")
	if len(series) != 1 || len(series[0].Points) != 1 {
		t.Fatalf("series=%+v", series)
	}
	pt := series[0].Points[0]
	if pt.Rate.Cmp(rat("31.50")) != 0 {
		t.Fatalf("rate=%s", pt.Rate.FloatString(2))
	}
	if pt.Metadata["source"] != "second" {
		t.Fatalf("meta=%v", pt.Metadata)
	}
}

func TestRateWalkBack(t *testing.T) {
	db := NewDB()
	db.Add("USD", "BRL", time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC), rat("5.00"))
	db.Add("USD", "BRL", time.Date(2023, 7, 10, 0, 0, 0, 0, time.UTC), rat("5.10"))
	r, dt, ok := db.Rate("USD", "BRL", time.Date(2023, 7, 5, 0, 0, 0, 0, time.UTC))
	if !ok || r.Cmp(rat("5.00")) != 0 || !dt.Equal(time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("rate=%v dt=%v ok=%v", r, dt, ok)
	}
	_, _, ok = db.Rate("USD", "BRL", time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC))
	if ok {
		t.Fatal("expected no rate before first observation")
	}
}

func TestAllSeriesAndPairs(t *testing.T) {
	db := NewDB()
	db.Add("USD", "BRL", time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), rat("5"))
	db.Add("B3_PETR4", "BRL", time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), rat("30"))
	db.Add("B3_PETR4", "BRL", time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC), rat("32"))
	all := db.AllSeries()
	if len(all) != 2 {
		t.Fatalf("all=%d", len(all))
	}
	// sorted by pair key: B3_PETR4|BRL then USD|BRL
	if all[0].Base != "B3_PETR4" || all[1].Base != "USD" {
		t.Fatalf("order: %s then %s", all[0].Base, all[1].Base)
	}
	if len(all[0].Points) != 2 {
		t.Fatalf("petr points=%d", len(all[0].Points))
	}
	pairs := db.Pairs()
	if len(pairs) != 2 || pairs[0].Base != "B3_PETR4" {
		t.Fatalf("pairs=%v", pairs)
	}
}

func TestLoadFileExample(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "example", "prices.beancount")
	db, diags, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags: %v", diags)
	}
	// last-wins on 2023-07-15 B3_PETR4
	r, _, ok := db.Rate("B3_PETR4", "BRL", time.Date(2023, 7, 15, 0, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("no rate")
	}
	if r.Cmp(rat("31.50")) != 0 {
		t.Fatalf("expected last-wins 31.50, got %s", r.FloatString(2))
	}
	// metadata from last same-day write
	for _, s := range db.SeriesForBase("B3_PETR4") {
		if s.Quote != "BRL" {
			continue
		}
		for _, p := range s.Points {
			if p.Date.Equal(time.Date(2023, 7, 15, 0, 0, 0, 0, time.UTC)) {
				if p.Metadata["source"] != "B3" {
					t.Fatalf("meta=%v", p.Metadata)
				}
			}
		}
	}
	pairs := db.Pairs()
	if len(pairs) < 5 {
		t.Fatalf("expected several pairs, got %d", len(pairs))
	}
}

func rat(s string) *big.Rat {
	r := new(big.Rat)
	if _, ok := r.SetString(s); !ok {
		panic(s)
	}
	return r
}

func TestRateIntermediateHop(t *testing.T) {
	db := NewDB()
	d := time.Date(2023, 7, 4, 0, 0, 0, 0, time.UTC)
	db.Add("SPDW", "USD", d, rat("34.942"))
	db.Add("USD", "BRL", d, rat("5.0563"))
	r, _, ok := db.Rate("SPDW", "BRL", d)
	if !ok {
		t.Fatal("expected hop SPDW→USD→BRL")
	}
	want := new(big.Rat).Mul(rat("34.942"), rat("5.0563"))
	if r.Cmp(want) != 0 {
		t.Fatalf("got %s want %s", r.FloatString(6), want.FloatString(6))
	}
}

func TestRateInverse(t *testing.T) {
	db := NewDB()
	d := time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC)
	db.Add("USD", "BRL", d, rat("5"))
	r, _, ok := db.Rate("BRL", "USD", d)
	if !ok || r.Cmp(rat("0.2")) != 0 {
		t.Fatalf("inverse got %v ok=%v", r, ok)
	}
}
