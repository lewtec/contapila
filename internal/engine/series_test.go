package engine

import (
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/period"
)

func TestExampleNetWorthSeries(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "example")
	p, pdb, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(p, pdb, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if l.OpCurrency == "" {
		t.Fatal("expected op currency")
	}
	pts, err := l.NetWorthSeries(time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) < 2 {
		t.Fatalf("expected multiple NW points, got %d", len(pts))
	}
	// monotonic dates
	for i := 1; i < len(pts); i++ {
		if !pts[i].Date.After(pts[i-1].Date) && !pts[i].Date.Equal(pts[i-1].Date) {
			t.Fatalf("dates not ordered")
		}
	}
}

func TestAccountSeriesUsesPadDate(t *testing.T) {
	dirs := []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Expenses:Food"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Equity:Opening"},
		ast.Pad{Meta: ast.Meta{Date: d("2020-01-01"), File: "t", Line: 4}, Account: "Assets:Cash", FromAccount: "Equity:Opening"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-10"), File: "t", Line: 5},
			Flag: "*", Narration: "Lunch",
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: &ast.Amount{Number: r("-30"), Commodity: "BRL"}},
				{Account: "Expenses:Food", Units: &ast.Amount{Number: r("30"), Commodity: "BRL"}},
			},
		},
		ast.Balance{Meta: ast.Meta{Date: d("2020-02-01"), File: "t", Line: 10}, Account: "Assets:Cash", Amount: ast.Amount{Number: r("100"), Commodity: "BRL"}},
	}
	expanded, diags := booking.ExpandPads(dirs, nil)
	if diags.HasErrors() {
		t.Fatalf("errors: %v", diags)
	}
	book := booking.New()
	book.Book(expanded)
	if book.Diags.HasErrors() {
		t.Fatalf("errors: %v", book.Diags)
	}
	l := &Ledger{
		Dirs:       expanded,
		Book:       book,
		OpCurrency: "BRL",
	}

	pts, err := l.AccountSeries("Assets:Cash", d("2020-01-01"), d("2020-02-01"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) == 0 {
		t.Fatal("expected account series points")
	}
	if !pts[0].Date.Equal(d("2020-01-01")) {
		t.Fatalf("first point=%s want 2020-01-01", pts[0].Date.Format("2006-01-02"))
	}
	if pts[0].Value.Cmp(r("130")) != 0 {
		t.Fatalf("first value=%s want 130", pts[0].Value.FloatString(4))
	}
	for _, pt := range pts {
		if pt.Value.Sign() < 0 {
			t.Fatalf("negative point on %s: %s", pt.Date.Format("2006-01-02"), pt.Value.FloatString(4))
		}
	}
}

func TestTrimZeroEdgeSeries(t *testing.T) {
	z := big.NewRat(0, 1)
	n := big.NewRat(10, 1)
	pts := []SeriesPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Value: z},
		{Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), Value: z},
		{Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Value: n},
		{Date: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC), Value: z}, // interior zero kept
		{Date: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), Value: n},
		{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Value: z},
	}
	got := trimZeroEdgeSeries(pts)
	// drop Jan–Feb leading zeros + June trailing zero → Mar, Apr(0), May
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	if !got[0].Date.Equal(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("first=%s", got[0].Date.Format("2006-01-02"))
	}
	if !got[len(got)-1].Date.Equal(time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("last=%s", got[len(got)-1].Date.Format("2006-01-02"))
	}
	// interior zero preserved
	if got[1].Value.Sign() != 0 {
		t.Fatal("expected interior zero kept")
	}
}

// Net worth series must revalue on PriceDB days even without balance-changing txns.
func TestNetWorthSeriesIncludesPriceDays(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "example")
	p, pdb, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(p, pdb, "personal")
	if err != nil {
		t.Fatal(err)
	}
	// 2023-07-04 has prices (USD, SPDW, BTC, …) in example prices.beancount.
	priceDay := time.Date(2023, 7, 4, 0, 0, 0, 0, time.UTC)
	// Window that may include fewer txns than price observations.
	from := time.Date(2023, 7, 3, 0, 0, 0, 0, time.UTC)
	to := time.Date(2023, 7, 5, 0, 0, 0, 0, time.UTC)
	pts, err := l.NetWorthSeries(from, to)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, pt := range pts {
		if pt.Date.Equal(priceDay) {
			found = true
			if pt.Value == nil {
				t.Fatal("nil value on price day")
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected NW point on price day %s; got %d points", priceDay.Format("2006-01-02"), len(pts))
	}
}

func TestExamplePnLBarsYear(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "example")
	p, pdb, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(p, pdb, "personal")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	bars := l.PnLBars(from, to, period.BinMonth)
	if len(bars) != 12 {
		t.Fatalf("months=%d", len(bars))
	}
}
