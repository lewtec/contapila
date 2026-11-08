package booking

import (
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/prices"
)

func TestExpandDatedCostsInjectsPrice(t *testing.T) {
	d := time.Date(2020, 1, 10, 0, 0, 0, 0, time.UTC)
	dirs := []ast.Directive{
		ast.Transaction{
			Meta: ast.Meta{Date: d, File: "t", Line: 1},
			Flag: "*",
			Postings: []ast.Posting{
				{
					Account: "Assets:Broker",
					Units:   amt("10", "HOOL"),
					Cost:    &ast.CostSpec{Number: r("100"), Commodity: "USD", Date: d},
				},
				{Account: "Assets:Cash", Units: amt("-1000", "USD")},
			},
		},
	}
	db := prices.NewDB()
	out := ExpandDatedCosts(dirs, db)
	var nPrice int
	for _, x := range out {
		if p, ok := x.(ast.Price); ok {
			nPrice++
			if p.Currency != "HOOL" || p.Amount.Commodity != "USD" {
				t.Fatalf("price=%+v", p)
			}
			if p.Amount.Number.Cmp(r("100")) != 0 {
				t.Fatalf("rate=%s", p.Amount.Number.FloatString(2))
			}
		}
	}
	if nPrice != 1 {
		t.Fatalf("prices=%d", nPrice)
	}
	rate, _, ok := db.Rate("HOOL", "USD", d)
	if !ok || rate.Cmp(r("100")) != 0 {
		t.Fatalf("db rate ok=%v %v", ok, rate)
	}
}
