package parser

import (
	"math/big"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/ast"
)

func TestParseBareNumberMissingCommodity(t *testing.T) {
	src := []byte(`
2020-01-01 open Assets:Cash
2020-01-01 open Expenses:Food
2020-01-02 * "no ccy"
  Expenses:Food  341.02
  Assets:Cash
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if !diags.HasErrors() {
		t.Fatalf("expected missing commodity error, diags=%v dirs=%d", diags, len(dirs))
	}
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "missing commodity") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diags=%v", diags)
	}
	// Must not leave Units nil (residual) for the food leg.
	for _, d := range dirs {
		txn, ok := d.(ast.Transaction)
		if !ok {
			continue
		}
		for _, p := range txn.Postings {
			if p.Account == "Expenses:Food" && p.Units == nil {
				t.Fatal("bare number treated as residual (Units==nil)")
			}
		}
	}
}

func TestParseBinaryExprAmount(t *testing.T) {
	src := []byte(`
2020-01-01 open Assets:Cash
2020-01-01 open Equity:Open
2020-01-02 * "expr"
  Assets:Cash  (19.2 - 3.07) USD
  Equity:Open
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags=%v", diags)
	}
	var got *big.Rat
	for _, d := range dirs {
		txn, ok := d.(ast.Transaction)
		if !ok {
			continue
		}
		for _, p := range txn.Postings {
			if p.Account == "Assets:Cash" && p.Units != nil {
				got = p.Units.Number
				if p.Units.Commodity != "USD" {
					t.Fatalf("ccy=%s", p.Units.Commodity)
				}
			}
		}
	}
	want := new(big.Rat)
	want.SetString("16.13")
	if got == nil || got.Cmp(want) != 0 {
		t.Fatalf("got %v want %s", got, want.FloatString(4))
	}
}

func TestParseNestedExprAmount(t *testing.T) {
	src := []byte(`
2020-01-01 open Assets:Cash
2020-01-01 open Equity:Open
2020-01-02 * "nested"
  Assets:Cash  -(10 + 2.5) * 2 USD
  Equity:Open
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags=%v", diags)
	}
	// -(12.5)*2 = -25
	want := big.NewRat(-25, 1)
	for _, d := range dirs {
		txn, ok := d.(ast.Transaction)
		if !ok {
			continue
		}
		for _, p := range txn.Postings {
			if p.Account == "Assets:Cash" && p.Units != nil {
				if p.Units.Number.Cmp(want) != 0 {
					t.Fatalf("got %s want %s", p.Units.Number.FloatString(4), want.FloatString(4))
				}
				return
			}
		}
	}
	t.Fatal("no cash posting")
}

func TestParseCostDate(t *testing.T) {
	src := []byte(`
2020-01-01 open Assets:Broker
2020-01-01 open Assets:Cash
2020-01-10 * "buy"
  Assets:Broker  10 HOOL {100.00 USD, 2020-01-10}
  Assets:Cash
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags=%v", diags)
	}
	for _, d := range dirs {
		txn, ok := d.(ast.Transaction)
		if !ok {
			continue
		}
		for _, p := range txn.Postings {
			if p.Account == "Assets:Broker" {
				if p.Cost == nil || p.Cost.Number == nil {
					t.Fatalf("cost=%+v", p.Cost)
				}
				if p.Cost.Date.IsZero() || p.Cost.Date.Format("2006-01-02") != "2020-01-10" {
					t.Fatalf("cost date=%v", p.Cost.Date)
				}
				if p.Cost.Commodity != "USD" {
					t.Fatalf("cost ccy=%s", p.Cost.Commodity)
				}
				return
			}
		}
	}
	t.Fatal("no broker posting")
}
