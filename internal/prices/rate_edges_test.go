package prices

import (
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/filesys"
)

func TestRateSameCommodityIsOne(t *testing.T) {
	db := NewDB()
	asOf := time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC)
	r, dt, ok := db.Rate("BRL", "BRL", asOf)
	if !ok {
		t.Fatal("expected identity rate")
	}
	if r.Cmp(big.NewRat(1, 1)) != 0 {
		t.Fatalf("rate=%s want 1", r.RatString())
	}
	if !dt.Equal(asOf) {
		t.Fatalf("date=%v want asOf", dt)
	}
}

func TestAddPriceSkipsIncomplete(t *testing.T) {
	d := time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC)
	db := NewDB()
	// nil number
	db.AddPrice(ast.Price{
		Meta:     ast.Meta{Date: d},
		Currency: "USD",
		Amount:   ast.Amount{Number: nil, Commodity: "BRL"},
	})
	// empty quote commodity
	db.AddPrice(ast.Price{
		Meta:     ast.Meta{Date: d},
		Currency: "USD",
		Amount:   ast.Amount{Number: rat("5"), Commodity: ""},
	})
	// empty base currency
	db.AddPrice(ast.Price{
		Meta:     ast.Meta{Date: d},
		Currency: "",
		Amount:   ast.Amount{Number: rat("5"), Commodity: "BRL"},
	})
	if pairs := db.Pairs(); len(pairs) != 0 {
		t.Fatalf("expected no pairs after incomplete AddPrice, got %v", pairs)
	}
}

func TestRateInverseZeroRejected(t *testing.T) {
	db := NewDB()
	d := time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC)
	db.Add("USD", "BRL", d, big.NewRat(0, 1))
	// Inverse of zero rate must not divide by zero.
	_, _, ok := db.Rate("BRL", "USD", d)
	if ok {
		t.Fatal("expected inverse of zero rate to fail")
	}
	// Direct zero rate is still returned.
	r, _, ok := db.Rate("USD", "BRL", d)
	if !ok || r.Sign() != 0 {
		t.Fatalf("direct zero rate got %v ok=%v", r, ok)
	}
}

func TestRateHopUsesEarlierLegDate(t *testing.T) {
	db := NewDB()
	d1 := time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2023, 7, 10, 0, 0, 0, 0, time.UTC)
	asOf := time.Date(2023, 7, 15, 0, 0, 0, 0, time.UTC)
	db.Add("SPDW", "USD", d1, rat("2"))
	db.Add("USD", "BRL", d2, rat("5"))
	r, dt, ok := db.Rate("SPDW", "BRL", asOf)
	if !ok {
		t.Fatal("expected hop")
	}
	want := new(big.Rat).Mul(rat("2"), rat("5"))
	if r.Cmp(want) != 0 {
		t.Fatalf("rate=%s want %s", r.FloatString(4), want.FloatString(4))
	}
	// Observation date is min(leg dates) so UI/staleness uses the older leg.
	if !dt.Equal(d1) {
		t.Fatalf("hop date=%v want earlier leg %v", dt, d1)
	}
}

func TestLoadFileFSMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-prices.beancount")
	db, _, err := LoadFileFS(filesys.OS{}, path)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if db == nil {
		t.Fatal("expected non-nil DB even on error")
	}
}

func TestLoadFileFSParsesPrices(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prices.beancount")
	const body = `2023-07-01 price USD 5.00 BRL
2023-07-01 price EUR 5.50 BRL
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	db, diags, err := LoadFileFS(filesys.OS{}, path)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags: %v", diags)
	}
	r, _, ok := db.Rate("USD", "BRL", time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC))
	if !ok || r.Cmp(rat("5")) != 0 {
		t.Fatalf("USD/BRL=%v ok=%v", r, ok)
	}
	r, _, ok = db.Rate("EUR", "BRL", time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC))
	if !ok || r.Cmp(rat("5.50")) != 0 {
		t.Fatalf("EUR/BRL=%v ok=%v", r, ok)
	}
	// non-price directives should not poison the DB
	path2 := filepath.Join(dir, "mixed.beancount")
	const mixed = `2023-01-01 open Assets:Cash BRL
2023-07-01 price USD 4.50 BRL
`
	if err := os.WriteFile(path2, []byte(mixed), 0o644); err != nil {
		t.Fatal(err)
	}
	db2, diags2, err := LoadFileFS(filesys.OS{}, path2)
	if err != nil {
		t.Fatal(err)
	}
	if diags2.HasErrors() {
		t.Fatalf("diags: %v", diags2)
	}
	r, _, ok = db2.Rate("USD", "BRL", time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC))
	if !ok || r.Cmp(rat("4.50")) != 0 {
		t.Fatalf("mixed USD/BRL=%v ok=%v", r, ok)
	}
}

func TestSeriesForBaseEmpty(t *testing.T) {
	db := NewDB()
	if got := db.SeriesForBase("NOPE"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	db.Add("USD", "BRL", time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), rat("5"))
	if got := db.SeriesForBase("EUR"); len(got) != 0 {
		t.Fatalf("EUR series=%v", got)
	}
	if got := db.SeriesForBase("USD"); len(got) != 1 || got[0].Quote != "BRL" {
		t.Fatalf("USD series=%+v", got)
	}
}
