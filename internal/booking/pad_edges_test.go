package booking

import (
	"testing"

	"github.com/lucasew/contapila-go/internal/ast"
)

// ExpandPads materializes pad→balance differences as synthetic "P" txns dated at
// the pad. These edges guard the skip paths that keep reports free of no-op pads.

func TestExpandPadsSkipsSameDayPadAndBalance(t *testing.T) {
	// Pad and balance on the same calendar day: Beancount books the pad amount at
	// the balance check, not as a prior-day synthetic txn.
	dirs := []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Equity:Opening"},
		ast.Pad{
			Meta:        ast.Meta{Date: d("2020-02-01"), File: "t", Line: 3},
			Account:     "Assets:Cash",
			FromAccount: "Equity:Opening",
		},
		ast.Balance{
			Meta:    ast.Meta{Date: d("2020-02-01"), File: "t", Line: 4},
			Account: "Assets:Cash",
			Amount:  ast.Amount{Number: r("100"), Commodity: "BRL"},
		},
	}
	expanded, diags := ExpandPads(dirs, nil)
	if diags.HasErrors() {
		t.Fatalf("errors: %v", diags)
	}
	if n := countPadTxns(expanded); n != 0 {
		t.Fatalf("same-day pad+balance: synthetic pads=%d want 0", n)
	}
	if len(expanded) != len(dirs) {
		t.Fatalf("len expanded=%d want %d", len(expanded), len(dirs))
	}
}

func TestExpandPadsSkipsWhenWithinTolerance(t *testing.T) {
	// Balance already matches inventory (diff 0) → no synthetic pad txn.
	dirs := []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Equity:Opening"},
		ast.Transaction{
			Meta:      ast.Meta{Date: d("2020-01-05"), File: "t", Line: 3},
			Flag:      "*",
			Narration: "seed",
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: amt("50", "BRL")},
				{Account: "Equity:Opening", Units: amt("-50", "BRL")},
			},
		},
		ast.Pad{
			Meta:        ast.Meta{Date: d("2020-01-10"), File: "t", Line: 10},
			Account:     "Assets:Cash",
			FromAccount: "Equity:Opening",
		},
		ast.Balance{
			Meta:    ast.Meta{Date: d("2020-02-01"), File: "t", Line: 20},
			Account: "Assets:Cash",
			Amount:  ast.Amount{Number: r("50"), Commodity: "BRL"},
		},
	}
	expanded, diags := ExpandPads(dirs, nil)
	if diags.HasErrors() {
		t.Fatalf("errors: %v", diags)
	}
	if n := countPadTxns(expanded); n != 0 {
		t.Fatalf("balanced account: synthetic pads=%d want 0", n)
	}
}

func TestExpandPadsSkipsWithoutPadDirective(t *testing.T) {
	// Balance assertion fails inventory but no pad → no synthesis (booking will warn).
	dirs := []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Equity:Opening"},
		ast.Balance{
			Meta:    ast.Meta{Date: d("2020-02-01"), File: "t", Line: 4},
			Account: "Assets:Cash",
			Amount:  ast.Amount{Number: r("100"), Commodity: "BRL"},
		},
	}
	expanded, diags := ExpandPads(dirs, nil)
	if diags.HasErrors() {
		t.Fatalf("errors: %v", diags)
	}
	if n := countPadTxns(expanded); n != 0 {
		t.Fatalf("no pad directive: synthetic pads=%d want 0", n)
	}
	if len(expanded) != len(dirs) {
		t.Fatalf("len expanded=%d want %d", len(expanded), len(dirs))
	}
}

func TestExpandPadsNegDiffCreditsAccount(t *testing.T) {
	// Inventory above asserted balance → synthetic posts negative units to account.
	dirs := []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Equity:Opening"},
		ast.Transaction{
			Meta:      ast.Meta{Date: d("2020-01-05"), File: "t", Line: 3},
			Flag:      "*",
			Narration: "seed",
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: amt("200", "BRL")},
				{Account: "Equity:Opening", Units: amt("-200", "BRL")},
			},
		},
		ast.Pad{
			Meta:        ast.Meta{Date: d("2020-01-10"), File: "t", Line: 10},
			Account:     "Assets:Cash",
			FromAccount: "Equity:Opening",
		},
		ast.Balance{
			Meta:    ast.Meta{Date: d("2020-02-01"), File: "t", Line: 20},
			Account: "Assets:Cash",
			Amount:  ast.Amount{Number: r("150"), Commodity: "BRL"},
		},
	}
	expanded, diags := ExpandPads(dirs, nil)
	if diags.HasErrors() {
		t.Fatalf("errors: %v", diags)
	}
	pads := padTxns(expanded)
	if len(pads) != 1 {
		t.Fatalf("synthetic pads=%d want 1", len(pads))
	}
	if !pads[0].Date.Equal(d("2020-01-10")) {
		t.Fatalf("pad date=%s want 2020-01-10", pads[0].Date.Format("2006-01-02"))
	}
	// Account side: asserted - actual = 150 - 200 = -50
	if got := pads[0].Postings[0].Units.Number; got.Cmp(r("-50")) != 0 {
		t.Fatalf("account pad units=%s want -50", got.FloatString(4))
	}
	if pads[0].Postings[0].Account != "Assets:Cash" {
		t.Fatalf("account=%s", pads[0].Postings[0].Account)
	}
	if got := pads[0].Postings[1].Units.Number; got.Cmp(r("50")) != 0 {
		t.Fatalf("from pad units=%s want 50", got.FloatString(4))
	}
	if pads[0].Postings[1].Account != "Equity:Opening" {
		t.Fatalf("from=%s", pads[0].Postings[1].Account)
	}

	e := New()
	e.Book(expanded)
	if e.Diags.HasErrors() {
		t.Fatalf("book errors: %v", e.Diags)
	}
	if got := e.balOf("Assets:Cash", "BRL"); got.Cmp(r("150")) != 0 {
		t.Fatalf("final balance=%s want 150", got.FloatString(4))
	}
}

func countPadTxns(dirs []ast.Directive) int {
	return len(padTxns(dirs))
}

func padTxns(dirs []ast.Directive) []ast.Transaction {
	var out []ast.Transaction
	for _, dir := range dirs {
		if txn, ok := dir.(ast.Transaction); ok && txn.Flag == "P" && txn.Narration == "pad" {
			out = append(out, txn)
		}
	}
	return out
}
