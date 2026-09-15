package booking

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
)

func d(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", s, time.UTC)
	return t
}
func r(s string) *big.Rat {
	x, ok := new(big.Rat).SetString(s)
	if !ok {
		panic(s)
	}
	return x
}
func amt(n, c string) *ast.Amount {
	return &ast.Amount{Number: r(n), Commodity: c}
}

func TestResidualCash(t *testing.T) {
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Expenses:Food"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-02"), File: "t"},
			Flag: "*", Narration: "Lunch",
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: amt("-30", "BRL")},
				{Account: "Expenses:Food"},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("errors: %v", e.Diags)
	}
	got := e.balOf("Expenses:Food", "BRL")
	if got.Cmp(r("30")) != 0 {
		t.Fatalf("got %s", got.FloatString(4))
	}
}

func TestExpandPadsDatesSyntheticTxnAtPad(t *testing.T) {
	dirs := []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Expenses:Food"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Equity:Opening"},
		ast.Pad{Meta: ast.Meta{Date: d("2020-01-01"), File: "t", Line: 4}, Account: "Assets:Cash", FromAccount: "Equity:Opening"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-10"), File: "t", Line: 5},
			Flag: "*", Narration: "Lunch",
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: amt("-30", "BRL")},
				{Account: "Expenses:Food", Units: amt("30", "BRL")},
			},
		},
		ast.Balance{Meta: ast.Meta{Date: d("2020-02-01"), File: "t", Line: 10}, Account: "Assets:Cash", Amount: ast.Amount{Number: r("100"), Commodity: "BRL"}},
	}

	expanded, diags := ExpandPads(dirs, nil)
	if diags.HasErrors() {
		t.Fatalf("errors: %v", diags)
	}
	var pads []ast.Transaction
	for _, dir := range expanded {
		if txn, ok := dir.(ast.Transaction); ok && txn.Flag == "P" && txn.Narration == "pad" {
			pads = append(pads, txn)
		}
	}
	if len(pads) != 1 {
		t.Fatalf("synthetic pad txns=%d want 1", len(pads))
	}
	if !pads[0].Date.Equal(d("2020-01-01")) {
		t.Fatalf("pad txn date=%s want 2020-01-01", pads[0].Date.Format("2006-01-02"))
	}
	if got := pads[0].Postings[0].Units.Number; got.Cmp(r("130")) != 0 {
		t.Fatalf("pad amount=%s want 130", got.FloatString(4))
	}

	e := New()
	e.Book(expanded)
	if e.Diags.HasErrors() {
		t.Fatalf("errors: %v", e.Diags)
	}
	if got := e.balOf("Assets:Cash", "BRL"); got.Cmp(r("100")) != 0 {
		t.Fatalf("balance=%s want 100", got.FloatString(4))
	}
	var bookedPads int
	for _, txn := range e.Txns {
		if txn.Txn.Flag == "P" && txn.Txn.Narration == "pad" {
			bookedPads++
		}
	}
	if bookedPads != 1 {
		t.Fatalf("booked pad txns=%d want 1", bookedPads)
	}
}

func TestResidualMultiCommodity(t *testing.T) {
	// One empty residual absorbs every leftover commodity.
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash:BRL"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash:USD"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Equity:Opening"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-02"), File: "t"},
			Flag: "*", Narration: "seed",
			Postings: []ast.Posting{
				{Account: "Assets:Cash:BRL", Units: amt("100", "BRL")},
				{Account: "Assets:Cash:USD", Units: amt("20", "USD")},
				{Account: "Equity:Opening"},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("errors: %v", e.Diags)
	}
	if got := e.balOf("Equity:Opening", "BRL"); got.Cmp(r("-100")) != 0 {
		t.Fatalf("BRL residual got %s", got.FloatString(4))
	}
	if got := e.balOf("Equity:Opening", "USD"); got.Cmp(r("-20")) != 0 {
		t.Fatalf("USD residual got %s", got.FloatString(4))
	}
	if len(e.Txns) != 1 {
		t.Fatalf("txns=%d", len(e.Txns))
	}
	// Source has 3 postings; residual expands to 2 filled amounts → 4 booked legs.
	if n := len(e.Txns[0].Postings); n != 4 {
		t.Fatalf("filled postings=%d want 4", n)
	}
	var brl, usd bool
	for _, fp := range e.Txns[0].Postings {
		if fp.Account != "Equity:Opening" || fp.Units == nil {
			continue
		}
		switch fp.Units.Commodity {
		case "BRL":
			brl = fp.Units.Number.Cmp(r("-100")) == 0
		case "USD":
			usd = fp.Units.Number.Cmp(r("-20")) == 0
		}
	}
	if !brl || !usd {
		t.Fatalf("filled residual legs missing: brl=%v usd=%v posts=%+v", brl, usd, e.Txns[0].Postings)
	}
}

func TestUnbalanced(t *testing.T) {
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-02"), File: "t"},
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: amt("-30", "BRL")},
			},
		},
	})
	if !e.Diags.HasErrors() {
		t.Fatal("expected error")
	}
}

func TestOversellUsesTxnNetNotSinglePosting(t *testing.T) {
	// Same txn: sell then buy same account+commodity. Per-posting check would
	// oversell; whole-txn net is zero relative to start+buys.
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Broker:X"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-02-01"), File: "t", Line: 3}, Flag: "*",
			Postings: []ast.Posting{
				// reduce first in source order (would fail if checked alone with 0 inventory)
				{Account: "Assets:Broker:X", Units: amt("-5", "X"), Cost: &ast.CostSpec{Number: r("10"), Commodity: "BRL"}},
				{Account: "Assets:Broker:X", Units: amt("5", "X"), Cost: &ast.CostSpec{Number: r("10"), Commodity: "BRL"}},
				{Account: "Assets:Cash", Units: amt("0", "BRL")},
			},
		},
	})
	for _, d := range e.Diags {
		if strings.Contains(d.Message, "oversell") {
			t.Fatalf("false oversell on net-zero inv txn: %v", e.Diags)
		}
	}
	if e.Diags.HasErrors() {
		t.Fatalf("errors: %v", e.Diags)
	}
	pos := e.getPos("Assets:Broker:X", "X")
	if pos == nil || pos.Units.Cmp(r("0")) != 0 {
		// buy 5 sell 5 from empty → end 0
		if pos == nil || pos.Units.Sign() != 0 {
			t.Fatalf("pos=%+v", pos)
		}
	}
}

func TestOversellIsWarning(t *testing.T) {
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Broker:X"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Income:Gains"},
		// hold 5
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-10"), File: "t"}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Broker:X", Units: amt("5", "X"), Cost: &ast.CostSpec{Number: r("10"), Commodity: "BRL"}},
				{Account: "Assets:Cash"},
			},
		},
		// sell 10 → oversell
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-02-01"), File: "t", Line: 10}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Broker:X", Units: amt("-10", "X"), Price: &ast.PriceSpec{Number: r("100"), Commodity: "BRL", Total: true}},
				{Account: "Assets:Cash", Units: amt("100", "BRL")},
				{Account: "Income:Gains"},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("oversell must not be a hard error: %v", e.Diags)
	}
	var warned bool
	for _, d := range e.Diags {
		if d.IsWarn() && strings.Contains(d.Message, "oversell") {
			warned = true
			if d.Line != 10 {
				t.Fatalf("line=%d", d.Line)
			}
		}
	}
	if !warned {
		t.Fatalf("expected oversell warn, diags=%v", e.Diags)
	}
	// inventory left intact (oversell not applied)
	pos := e.getPos("Assets:Broker:X", "X")
	if pos == nil || pos.Units.Cmp(r("5")) != 0 {
		t.Fatalf("units after oversell skip: %+v", pos)
	}
}

func TestAverageCostSell(t *testing.T) {
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Broker:X"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Income:Gains"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-10"), File: "t"}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Broker:X", Units: amt("10", "X"), Cost: &ast.CostSpec{Number: r("10"), Commodity: "BRL"}},
				{Account: "Assets:Cash"},
			},
		},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-02-10"), File: "t"}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Broker:X", Units: amt("10", "X"), Cost: &ast.CostSpec{Number: r("20"), Commodity: "BRL"}},
				{Account: "Assets:Cash"},
			},
		},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-03-10"), File: "t"}, Flag: "*", Narration: "Sell",
			Postings: []ast.Posting{
				{Account: "Assets:Broker:X", Units: amt("-5", "X"), Price: &ast.PriceSpec{Number: r("100"), Commodity: "BRL", Total: true}},
				{Account: "Assets:Cash", Units: amt("100", "BRL")},
				{Account: "Income:Gains"},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("errors: %v", e.Diags)
	}
	pos := e.getPos("Assets:Broker:X", "X")
	if pos == nil || pos.Units.Cmp(r("15")) != 0 {
		t.Fatalf("units left: %+v", pos)
	}
	// avg should still be 15
	if pos.Avg().Cmp(r("15")) != 0 {
		t.Fatalf("avg %s", pos.Avg().FloatString(4))
	}
	// gains: proceeds 100 - cost 75 = 25, income account gets -25 (credit)
	g := e.balOf("Income:Gains", "BRL")
	if g.Cmp(r("-25")) != 0 {
		t.Fatalf("gains bal %s want -25", g.FloatString(4))
	}
}

func TestSameDayOpenAfterTxnInStreamOK(t *testing.T) {
	// Same date: txn appears before open in stream (include/file order).
	// Type rank must still open accounts before booking the txn.
	e := New()
	e.Book([]ast.Directive{
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-01"), File: "txns.beancount", Line: 10},
			Flag: "*", Narration: "before open in stream",
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: amt("10", "BRL")},
				{Account: "Equity:Open", Units: amt("-10", "BRL")},
			},
		},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01"), File: "accounts.beancount", Line: 1}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01"), File: "accounts.beancount", Line: 2}, Account: "Equity:Open"},
	})
	for _, d := range e.Diags {
		if strings.Contains(d.Message, "not opened") || strings.Contains(d.Message, "before open") {
			t.Fatalf("unexpected: %v", d)
		}
	}
	if _, ok := e.Open["Assets:Cash"]; !ok {
		t.Fatal("open missing")
	}
}

func TestSameDayOpenBeforeTxnOK(t *testing.T) {
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Equity:Open"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-01"), File: "t", Line: 1},
			Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: amt("10", "BRL")},
				{Account: "Equity:Open", Units: amt("-10", "BRL")},
			},
		},
	})
	for _, d := range e.Diags {
		if strings.Contains(d.Message, "not opened") || strings.Contains(d.Message, "before open") {
			t.Fatalf("unexpected: %v", d)
		}
	}
}

func TestTxnBeforeOpenDateNotOpened(t *testing.T) {
	// Date sort books the earlier txn before the later open is registered,
	// so this surfaces as "not opened" (not "before open").
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-02-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-02-01")}, Account: "Equity:Open"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-15"), File: "t", Line: 1},
			Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: amt("10", "BRL")},
				{Account: "Equity:Open", Units: amt("-10", "BRL")},
			},
		},
	})
	var notOpened bool
	for _, d := range e.Diags {
		if strings.Contains(d.Message, "not opened") {
			notOpened = true
		}
	}
	if !notOpened {
		t.Fatalf("expected not opened for txn before open date, diags=%v", e.Diags)
	}
}

func TestBuyWithTotalPriceAsCost(t *testing.T) {
	// 10 X @@ 100 BRL → unit cost 10 BRL, inventory position established.
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Broker:X"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-10"), File: "t"}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Broker:X", Units: amt("10", "X"), Price: &ast.PriceSpec{Number: r("100"), Commodity: "BRL", Total: true}},
				{Account: "Assets:Cash", Units: amt("-100", "BRL")},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("errors: %v", e.Diags)
	}
	pos := e.getPos("Assets:Broker:X", "X")
	if pos == nil || pos.Units.Cmp(r("10")) != 0 {
		t.Fatalf("units: %+v", pos)
	}
	if pos.TotalCost.Cmp(r("100")) != 0 || pos.CostComm != "BRL" {
		t.Fatalf("cost: total=%s comm=%s", pos.TotalCost.FloatString(4), pos.CostComm)
	}
	if pos.Avg().Cmp(r("10")) != 0 {
		t.Fatalf("avg %s", pos.Avg().FloatString(4))
	}
}

func TestBuyWithUnitPriceAsCost(t *testing.T) {
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Broker:X"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-10"), File: "t"}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Broker:X", Units: amt("10", "X"), Price: &ast.PriceSpec{Number: r("10"), Commodity: "BRL", Total: false}},
				{Account: "Assets:Cash", Units: amt("-100", "BRL")},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("errors: %v", e.Diags)
	}
	pos := e.getPos("Assets:Broker:X", "X")
	if pos == nil || pos.Avg().Cmp(r("10")) != 0 {
		t.Fatalf("pos=%+v avg=%v", pos, pos)
	}
}

func TestSecondBuyWithPriceAfterCosted(t *testing.T) {
	// First buy with {...}, second with @@ — must merge average, not error.
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Broker:X"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-10"), File: "t"}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Broker:X", Units: amt("10", "X"), Cost: &ast.CostSpec{Number: r("10"), Commodity: "BRL"}},
				{Account: "Assets:Cash"},
			},
		},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-02-10"), File: "t"}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Broker:X", Units: amt("5", "X"), Price: &ast.PriceSpec{Number: r("75"), Commodity: "BRL", Total: true}},
				{Account: "Assets:Cash", Units: amt("-75", "BRL")},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("errors: %v", e.Diags)
	}
	pos := e.getPos("Assets:Broker:X", "X")
	if pos == nil || pos.Units.Cmp(r("15")) != 0 {
		t.Fatalf("units: %+v", pos)
	}
	// total cost 100 + 75 = 175; avg 175/15
	if pos.TotalCost.Cmp(r("175")) != 0 {
		t.Fatalf("total cost %s", pos.TotalCost.FloatString(4))
	}
	wantAvg := r("175/15")
	if pos.Avg().Cmp(wantAvg) != 0 {
		t.Fatalf("avg %s want %s", pos.Avg().FloatString(6), wantAvg.FloatString(6))
	}
}

func TestBuyPriceThenSellUsesAvgCost(t *testing.T) {
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Broker:X"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Income:Gains"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-10"), File: "t"}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Broker:X", Units: amt("10", "X"), Price: &ast.PriceSpec{Number: r("100"), Commodity: "BRL", Total: true}},
				{Account: "Assets:Cash"},
			},
		},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-03-10"), File: "t"}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Broker:X", Units: amt("-5", "X"), Price: &ast.PriceSpec{Number: r("80"), Commodity: "BRL", Total: true}},
				{Account: "Assets:Cash", Units: amt("80", "BRL")},
				{Account: "Income:Gains"},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("errors: %v", e.Diags)
	}
	// cost of 5 = 50; proceeds 80; gains credit -30
	if g := e.balOf("Income:Gains", "BRL"); g.Cmp(r("-30")) != 0 {
		t.Fatalf("gains %s", g.FloatString(4))
	}
}
