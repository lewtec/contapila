package booking

import (
	"fmt"
	"testing"

	"github.com/lucasew/contapila-go/internal/ast"
)

// Dense synthetic streams for Book / ExpandPads hot paths. Keep fixtures small
// enough for short -benchtime runs in CI; do not load kitchensink inventory.

func balancedCashStream(nTxn int) []ast.Directive {
	dirs := make([]ast.Directive, 0, nTxn+8)
	openDay := d("2020-01-01")
	for _, acct := range []string{
		"Assets:Cash",
		"Expenses:Food",
		"Income:Salary",
		"Equity:Opening",
	} {
		dirs = append(dirs, ast.Open{Meta: ast.Meta{Date: openDay}, Account: acct})
	}
	dirs = append(dirs, ast.Transaction{
		Meta:      ast.Meta{Date: openDay, File: "bench", Line: 1},
		Flag:      "*",
		Narration: "seed",
		Postings: []ast.Posting{
			{Account: "Assets:Cash", Units: amt("1000000", "BRL")},
			{Account: "Equity:Opening"},
		},
	})
	base := d("2020-01-02")
	for i := 0; i < nTxn; i++ {
		day := base.AddDate(0, 0, i%365)
		line := i + 2
		if i%2 == 0 {
			dirs = append(dirs, ast.Transaction{
				Meta:      ast.Meta{Date: day, File: "bench", Line: line},
				Flag:      "*",
				Narration: fmt.Sprintf("exp-%d", i),
				Postings: []ast.Posting{
					{Account: "Assets:Cash", Units: amt("-30", "BRL")},
					{Account: "Expenses:Food"},
				},
			})
			continue
		}
		dirs = append(dirs, ast.Transaction{
			Meta:      ast.Meta{Date: day, File: "bench", Line: line},
			Flag:      "*",
			Narration: fmt.Sprintf("inc-%d", i),
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: amt("50", "BRL")},
				{Account: "Income:Salary"},
			},
		})
	}
	return dirs
}

func avgCostInventoryStream(nRounds int) []ast.Directive {
	dirs := make([]ast.Directive, 0, nRounds*2+8)
	openDay := d("2020-01-01")
	for _, acct := range []string{
		"Assets:Broker:X",
		"Assets:Cash",
		"Income:Gains",
		"Equity:Opening",
	} {
		dirs = append(dirs, ast.Open{Meta: ast.Meta{Date: openDay}, Account: acct})
	}
	dirs = append(dirs, ast.Transaction{
		Meta:      ast.Meta{Date: openDay, File: "bench", Line: 1},
		Flag:      "*",
		Narration: "seed cash",
		Postings: []ast.Posting{
			{Account: "Assets:Cash", Units: amt("1000000", "BRL")},
			{Account: "Equity:Opening"},
		},
	})
	base := d("2020-01-02")
	line := 2
	for i := 0; i < nRounds; i++ {
		buyDay := base.AddDate(0, 0, i*2)
		// Unit cost steps so average cost work is non-trivial across rounds.
		unit := fmt.Sprintf("%d", 10+i%20)
		dirs = append(dirs, ast.Transaction{
			Meta:      ast.Meta{Date: buyDay, File: "bench", Line: line},
			Flag:      "*",
			Narration: fmt.Sprintf("buy-%d", i),
			Postings: []ast.Posting{
				{Account: "Assets:Broker:X", Units: amt("10", "X"), Cost: &ast.CostSpec{Number: r(unit), Commodity: "BRL"}},
				{Account: "Assets:Cash"},
			},
		})
		line++
		sellDay := buyDay.AddDate(0, 0, 1)
		dirs = append(dirs, ast.Transaction{
			Meta:      ast.Meta{Date: sellDay, File: "bench", Line: line},
			Flag:      "*",
			Narration: fmt.Sprintf("sell-%d", i),
			Postings: []ast.Posting{
				{Account: "Assets:Broker:X", Units: amt("-5", "X"), Price: &ast.PriceSpec{Number: r("100"), Commodity: "BRL", Total: true}},
				{Account: "Assets:Cash", Units: amt("100", "BRL")},
				{Account: "Income:Gains"},
			},
		})
		line++
	}
	return dirs
}

func padBalanceStream(nPads int) []ast.Directive {
	dirs := make([]ast.Directive, 0, nPads*4+4)
	openDay := d("2020-01-01")
	dirs = append(dirs,
		ast.Open{Meta: ast.Meta{Date: openDay}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: openDay}, Account: "Expenses:Food"},
		ast.Open{Meta: ast.Meta{Date: openDay}, Account: "Equity:Opening"},
	)
	base := d("2020-01-01")
	for i := 0; i < nPads; i++ {
		// Distinct accounts so each pad/balance pair is independent.
		acct := fmt.Sprintf("Assets:Cash:Pad%d", i)
		dirs = append(dirs, ast.Open{Meta: ast.Meta{Date: openDay}, Account: acct})
		padDay := base.AddDate(0, 0, i)
		balDay := padDay.AddDate(0, 1, 0)
		// Spend so pad has a non-zero correction to the later balance.
		dirs = append(dirs,
			ast.Pad{
				Meta:        ast.Meta{Date: padDay, File: "bench", Line: i*3 + 1},
				Account:     acct,
				FromAccount: "Equity:Opening",
			},
			ast.Transaction{
				Meta:      ast.Meta{Date: padDay.AddDate(0, 0, 1), File: "bench", Line: i*3 + 2},
				Flag:      "*",
				Narration: fmt.Sprintf("spend-%d", i),
				Postings: []ast.Posting{
					{Account: acct, Units: amt("-30", "BRL")},
					{Account: "Expenses:Food", Units: amt("30", "BRL")},
				},
			},
			ast.Balance{
				Meta:    ast.Meta{Date: balDay, File: "bench", Line: i*3 + 3},
				Account: acct,
				Amount:  ast.Amount{Number: r("100"), Commodity: "BRL"},
			},
		)
	}
	return dirs
}

func BenchmarkBook_balancedTxns(b *testing.B) {
	dirs := balancedCashStream(256)
	// Sanity once: residual fill + sort must succeed for the fixture.
	e0 := New()
	e0.Book(dirs)
	if e0.Diags.HasErrors() {
		b.Fatalf("fixture errors: %v", e0.Diags)
	}

	b.ReportAllocs()
	for b.Loop() {
		e := New()
		e.Book(dirs)
		if e.Diags.HasErrors() {
			b.Fatalf("errors: %v", e.Diags)
		}
	}
}

func BenchmarkBook_avgCostInventory(b *testing.B) {
	dirs := avgCostInventoryStream(64)
	e0 := New()
	e0.Book(dirs)
	if e0.Diags.HasErrors() {
		b.Fatalf("fixture errors: %v", e0.Diags)
	}

	b.ReportAllocs()
	for b.Loop() {
		e := New()
		e.Book(dirs)
		if e.Diags.HasErrors() {
			b.Fatalf("errors: %v", e.Diags)
		}
	}
}

func BenchmarkExpandPads(b *testing.B) {
	dirs := padBalanceStream(32)
	expanded0, diags0 := ExpandPads(dirs, nil)
	if diags0.HasErrors() {
		b.Fatalf("fixture expand errors: %v", diags0)
	}
	e0 := New()
	e0.Book(expanded0)
	if e0.Diags.HasErrors() {
		b.Fatalf("fixture book errors: %v", e0.Diags)
	}

	b.ReportAllocs()
	for b.Loop() {
		expanded, diags := ExpandPads(dirs, nil)
		if diags.HasErrors() {
			b.Fatalf("errors: %v", diags)
		}
		e := New()
		e.Book(expanded)
		if e.Diags.HasErrors() {
			b.Fatalf("book errors: %v", e.Diags)
		}
	}
}
