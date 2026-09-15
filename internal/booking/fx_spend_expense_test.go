package booking

import (
	"testing"

	"github.com/lucasew/contapila-go/internal/ast"
)

func TestUSDWithBRLCostPayExpenseResidual(t *testing.T) {
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:US:Nomad:ContaCorrente"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:BR:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Expenses:CustoFixo:Cloud:Storage"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2024-06-01"), File: "t"}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:US:Nomad:ContaCorrente", Units: amt("100", "USD"),
					Cost: &ast.CostSpec{Number: r("5"), Commodity: "BRL"}},
				{Account: "Assets:BR:Cash", Units: amt("-500", "BRL")},
			},
		},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2024-11-19"), File: "t", Line: 2}, Flag: "*",
			Payee: "Rsync.net", Narration: "200GB por um ano",
			Postings: []ast.Posting{
				{Account: "Expenses:CustoFixo:Cloud:Storage", Units: amt("19.20", "USD")},
				{Account: "Assets:US:Nomad:ContaCorrente"},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("book: %v", e.Diags)
	}
	if got := e.balOf("Expenses:CustoFixo:Cloud:Storage", "USD"); got.Cmp(r("19.20")) != 0 {
		t.Fatalf("expense %s", got.FloatString(4))
	}
	if got := e.balOf("Assets:US:Nomad:ContaCorrente", "USD"); got.Cmp(r("80.80")) != 0 {
		t.Fatalf("cash bal %s want 80.80", got.FloatString(4))
	}
	pos := e.getPos("Assets:US:Nomad:ContaCorrente", "USD")
	if pos == nil || pos.Units.Cmp(r("80.80")) != 0 {
		t.Fatalf("inv units %+v", pos)
	}
	if pos.TotalCost.Cmp(r("404")) != 0 {
		t.Fatalf("inv total cost %s want 404", pos.TotalCost.FloatString(4))
	}
}

func TestUSDDepositIntoExistingLotAtAverage(t *testing.T) {
	// ContaCorrente 19.20 USD into existing costed lot without braces → impute at avg.
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:US:Nomad:ContaCorrente"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:BR:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Expenses:Cloud"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2024-06-01"), File: "t"}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:US:Nomad:ContaCorrente", Units: amt("100", "USD"),
					Cost: &ast.CostSpec{Number: r("5"), Commodity: "BRL"}},
				{Account: "Assets:BR:Cash", Units: amt("-500", "BRL")},
			},
		},
		// amount on cash, residual expense (swapped vs Rsync — used to require cost)
		ast.Transaction{
			Meta: ast.Meta{Date: d("2024-11-19"), File: "t", Line: 2}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:US:Nomad:ContaCorrente", Units: amt("19.20", "USD")},
				{Account: "Expenses:Cloud"},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("book: %v", e.Diags)
	}
	pos := e.getPos("Assets:US:Nomad:ContaCorrente", "USD")
	if pos == nil || pos.Units.Cmp(r("119.20")) != 0 {
		t.Fatalf("units %+v", pos)
	}
	// 100*5 + 19.20*5 = 596 BRL total cost, avg still 5
	if pos.TotalCost.Cmp(r("596")) != 0 {
		t.Fatalf("cost %s", pos.TotalCost.FloatString(4))
	}
	if got := e.balOf("Expenses:Cloud", "USD"); got.Cmp(r("-19.20")) != 0 {
		t.Fatalf("expense residual %s", got.FloatString(4))
	}
}

func TestUSDWithBRLCostPayExpenseExplicit(t *testing.T) {
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:US:Nomad:ContaCorrente"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:BR:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Expenses:Cloud"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2024-06-01"), File: "t"}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:US:Nomad:ContaCorrente", Units: amt("100", "USD"),
					Cost: &ast.CostSpec{Number: r("5"), Commodity: "BRL"}},
				{Account: "Assets:BR:Cash", Units: amt("-500", "BRL")},
			},
		},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2024-11-19"), File: "t", Line: 2}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Expenses:Cloud", Units: amt("19.20", "USD")},
				{Account: "Assets:US:Nomad:ContaCorrente", Units: amt("-19.20", "USD")},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("book: %v", e.Diags)
	}
	if got := e.balOf("Assets:US:Nomad:ContaCorrente", "USD"); got.Cmp(r("80.80")) != 0 {
		t.Fatalf("cash %s", got.FloatString(4))
	}
}
