package booking

import (
	"testing"

	"github.com/lucasew/contapila-go/internal/ast"
)

func TestUSDWithBRLCostThenBuyStock(t *testing.T) {
	// Buy USD with BRL cost, then spend USD on stocks @ USD + fees.
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2025-01-01")}, Account: "Assets:US:BTG:Investimento"},
		ast.Open{Meta: ast.Meta{Date: d("2025-01-01")}, Account: "Assets:BR:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2025-01-01")}, Account: "Assets:US:BTG:Stock"},
		ast.Open{Meta: ast.Meta{Date: d("2025-01-01")}, Account: "Expenses:Taxa:BR:BTG:Corretagem"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2025-05-01"), File: "t", Line: 1}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:US:BTG:Investimento", Units: amt("200", "USD"),
					Cost: &ast.CostSpec{Number: r("5"), Commodity: "BRL"}},
				{Account: "Assets:BR:Cash", Units: amt("-1000", "BRL")},
			},
		},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2025-06-02"), File: "t", Line: 2}, Flag: "*",
			Narration: "Compra exterior",
			Postings: []ast.Posting{
				{Account: "Assets:US:BTG:Stock", Units: amt("0.67606547", "VNQ"),
					Price: &ast.PriceSpec{Number: r("88.7488"), Commodity: "USD", Total: false}},
				{Account: "Expenses:Taxa:BR:BTG:Corretagem", Units: amt("1", "USD")},
				{Account: "Assets:US:BTG:Stock", Units: amt("2", "SPDW"),
					Price: &ast.PriceSpec{Number: r("40.0688"), Commodity: "USD", Total: false}},
				{Account: "Assets:US:BTG:Stock", Units: amt("0.89502056", "SPDW"),
					Price: &ast.PriceSpec{Number: r("40.0688"), Commodity: "USD", Total: false}},
				{Account: "Expenses:Taxa:BR:BTG:Corretagem", Units: amt("1.25", "USD")},
				{Account: "Assets:US:BTG:Investimento", Units: amt("-178.249998998464", "USD")},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("book failed: %v", e.Diags)
	}
	posUSD := e.getPos("Assets:US:BTG:Investimento", "USD")
	if posUSD == nil {
		t.Fatal("expected remaining USD inventory")
	}
	// 200 - 178.249998998464 left
	wantLeft := r("200").Sub(r("200"), r("178.249998998464"))
	if posUSD.Units.Cmp(wantLeft) != 0 {
		t.Fatalf("USD units %s want %s", posUSD.Units.FloatString(8), wantLeft.FloatString(8))
	}
	// Cost basis reduced proportionally: spent 178.25/200 of 1000 BRL total cost
	if posUSD.CostComm != "BRL" {
		t.Fatalf("cost comm %s", posUSD.CostComm)
	}
	if pos := e.getPos("Assets:US:BTG:Stock", "SPDW"); pos == nil || pos.CostComm != "USD" {
		t.Fatalf("SPDW pos=%+v", pos)
	}
	if pos := e.getPos("Assets:US:BTG:Stock", "VNQ"); pos == nil || pos.CostComm != "USD" {
		t.Fatalf("VNQ pos=%+v", pos)
	}
}

func TestFXSellToBRLStillUsesCostWeight(t *testing.T) {
	// Selling USD for BRL must still use BRL cost basis weight so gains residual works.
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2025-01-01")}, Account: "Assets:USD"},
		ast.Open{Meta: ast.Meta{Date: d("2025-01-01")}, Account: "Assets:BRL"},
		ast.Open{Meta: ast.Meta{Date: d("2025-01-01")}, Account: "Income:Gains"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2025-05-01"), File: "t"}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:USD", Units: amt("100", "USD"),
					Cost: &ast.CostSpec{Number: r("5"), Commodity: "BRL"}},
				{Account: "Assets:BRL", Units: amt("-500", "BRL")},
			},
		},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2025-06-01"), File: "t", Line: 2}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:USD", Units: amt("-100", "USD")},
				{Account: "Assets:BRL", Units: amt("550", "BRL")},
				{Account: "Income:Gains"},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("%v", e.Diags)
	}
	// Cost 500 BRL out, proceeds 550 BRL → gains credit -50
	g := e.balOf("Income:Gains", "BRL")
	if g.Cmp(r("-50")) != 0 {
		t.Fatalf("gains %s want -50", g.FloatString(4))
	}
}
