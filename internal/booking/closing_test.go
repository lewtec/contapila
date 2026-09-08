package booking

import (
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
)

func bookProbe(dirs []ast.Directive) []BookedTxn {
	e := New()
	e.Book(dirs)
	return e.Txns
}

func TestExpandClosingHappyPath(t *testing.T) {
	dirs := []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:Pending:Refund"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Equity:Open"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2024-01-05"), File: "j.beancount", Line: 5},
			Flag: "*", Narration: "fund",
			Postings: []ast.Posting{
				{Account: "Assets:Pending:Refund", Units: amt("50", "BRL")},
				{Account: "Equity:Open", Units: amt("-50", "BRL")},
			},
		},
		ast.Transaction{
			Meta:      ast.Meta{Date: d("2024-01-10"), File: "j.beancount", Line: 12},
			Flag:      "*",
			Narration: "drain",
			Postings: []ast.Posting{
				{
					Account:  "Assets:Pending:Refund",
					Units:    amt("-50", "BRL"),
					Metadata: ast.Metadata{"closing": "TRUE"},
				},
				{Account: "Assets:Cash", Units: amt("50", "BRL")},
			},
		},
	}
	out, diags := ExpandClosing(dirs, bookProbe(dirs))
	if diags.HasErrors() {
		t.Fatalf("diags: %v", diags)
	}
	if len(out) != len(dirs)+2 {
		t.Fatalf("want +2 synth, got %d dirs (was %d)", len(out), len(dirs))
	}
	bal, ok := out[len(dirs)].(ast.Balance)
	if !ok || bal.Account != "Assets:Pending:Refund" || bal.Amount.Commodity != "BRL" || bal.Amount.Number.Sign() != 0 {
		t.Fatalf("balance=%+v", out[len(dirs)])
	}
	if !bal.Date.Equal(d("2024-01-11")) {
		t.Fatalf("balance date=%s", bal.Date.Format("2006-01-02"))
	}
	cl, ok := out[len(dirs)+1].(ast.Close)
	if !ok || cl.Account != "Assets:Pending:Refund" || !cl.Date.Equal(d("2024-01-11")) {
		t.Fatalf("close=%+v", out[len(dirs)+1])
	}
	e, _, bdiags := BookWithClosing(dirs, nil)
	if bdiags.HasErrors() {
		t.Fatalf("book: %v", bdiags)
	}
	if _, ok := e.Close["Assets:Pending:Refund"]; !ok {
		t.Fatal("expected close recorded")
	}
}

func TestExpandClosingResidualInferred(t *testing.T) {
	// Unit-less closing leg reuses residual fill from bookTxn.
	dirs := []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:Pending:Refund"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Equity:Open"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2024-01-05"), File: "j", Line: 1},
			Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Pending:Refund", Units: amt("50", "BRL")},
				{Account: "Equity:Open", Units: amt("-50", "BRL")},
			},
		},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2024-01-10"), File: "j", Line: 3},
			Flag: "*",
			Postings: []ast.Posting{
				// residual drain of pending → filled -50 BRL
				{Account: "Assets:Pending:Refund", Metadata: ast.Metadata{"closing": "TRUE"}},
				{Account: "Assets:Cash", Units: amt("50", "BRL")},
			},
		},
	}
	out, diags := ExpandClosing(dirs, bookProbe(dirs))
	if diags.HasErrors() {
		t.Fatalf("diags: %v", diags)
	}
	var bal ast.Balance
	found := false
	for _, d := range out {
		if b, ok := d.(ast.Balance); ok {
			bal = b
			found = true
		}
	}
	if !found || bal.Account != "Assets:Pending:Refund" || bal.Amount.Commodity != "BRL" {
		t.Fatalf("expected residual→BRL balance, got found=%v bal=%+v", found, bal)
	}
	e, _, bdiags := BookWithClosing(dirs, nil)
	if bdiags.HasErrors() {
		t.Fatalf("book: %v", bdiags)
	}
	if _, ok := e.Close["Assets:Pending:Refund"]; !ok {
		t.Fatal("expected close")
	}
}

func TestExpandClosingUnbookedError(t *testing.T) {
	// Two residuals → txn fails to book → closing cannot use residual fill.
	dirs := []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:A"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:B"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2024-01-10"), File: "j", Line: 3},
			Postings: []ast.Posting{
				{Account: "Assets:A", Metadata: ast.Metadata{"closing": "TRUE"}},
				{Account: "Assets:B"},
			},
		},
	}
	out, diags := ExpandClosing(dirs, bookProbe(dirs))
	if !diags.HasErrors() {
		t.Fatal("expected error")
	}
	if len(out) != len(dirs) {
		t.Fatalf("should not inject on error, got %d", len(out))
	}
	if !hasMsg(diags, "not booked") && !hasMsg(diags, "infer") {
		t.Fatalf("msg=%v", diags)
	}
}

func TestExpandClosingExistingCloseWarn(t *testing.T) {
	dirs := []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:Pending:Refund"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:Cash"},
		ast.Close{Meta: ast.Meta{Date: d("2024-01-20")}, Account: "Assets:Pending:Refund"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2024-01-10"), File: "j", Line: 5},
			Flag: "*",
			Postings: []ast.Posting{
				{
					Account:  "Assets:Pending:Refund",
					Units:    amt("-1", "BRL"),
					Metadata: ast.Metadata{"closing": "true"},
				},
				{Account: "Assets:Cash", Units: amt("1", "BRL")},
			},
		},
	}
	out, diags := ExpandClosing(dirs, bookProbe(dirs))
	if diags.HasErrors() {
		t.Fatalf("errors: %v", diags)
	}
	if !hasSeverity(diags, diag.Warn) {
		t.Fatalf("expected warn, got %v", diags)
	}
	var totalClose, totalBal int
	for _, d := range out {
		switch d.(type) {
		case ast.Close:
			totalClose++
		case ast.Balance:
			totalBal++
		}
	}
	if totalClose != 1 {
		t.Fatalf("closes=%d want only user close", totalClose)
	}
	if totalBal != 1 {
		t.Fatalf("balances=%d want 1 synth", totalBal)
	}
}

func TestExpandClosingOnlyMarkedCommodity(t *testing.T) {
	dirs := []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:Broker"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:Cash"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2024-02-01"), File: "j", Line: 1},
			Flag: "*",
			Postings: []ast.Posting{
				{
					Account:  "Assets:Broker",
					Units:    amt("-10", "USD"),
					Metadata: ast.Metadata{"closing": "TRUE"},
				},
				{Account: "Assets:Cash", Units: amt("50", "BRL")},
			},
		},
	}
	// unbalanced multi-ccy may fail book — use matching residual or same ccy
	// Force explicit units path: book may error on unbalanced BRL/USD.
	// Use residual cash for BRL only:
	dirs = []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:Broker"},
		ast.Open{Meta: ast.Meta{Date: d("2024-01-01")}, Account: "Assets:Cash"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2024-02-01"), File: "j", Line: 1},
			Flag: "*",
			Postings: []ast.Posting{
				{
					Account:  "Assets:Broker",
					Units:    amt("-10", "USD"),
					Metadata: ast.Metadata{"closing": "TRUE"},
				},
				{Account: "Assets:Cash", Units: amt("10", "USD")},
			},
		},
	}
	out, diags := ExpandClosing(dirs, bookProbe(dirs))
	if diags.HasErrors() {
		t.Fatalf("%v", diags)
	}
	var bals []ast.Balance
	for _, d := range out {
		if b, ok := d.(ast.Balance); ok {
			bals = append(bals, b)
		}
	}
	if len(bals) != 1 || bals[0].Amount.Commodity != "USD" {
		t.Fatalf("bals=%+v", bals)
	}
}

func hasMsg(list diag.List, sub string) bool {
	sub = strings.ToLower(sub)
	for _, d := range list {
		if strings.Contains(strings.ToLower(d.Message), sub) {
			return true
		}
	}
	return false
}

func hasSeverity(list diag.List, s diag.Severity) bool {
	for _, d := range list {
		if d.Severity == s {
			return true
		}
	}
	return false
}
