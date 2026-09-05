package engine

import (
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/pkg/project"
)

func TestInferOpCurrencyFromOption(t *testing.T) {
	dirs := []ast.Directive{
		ast.Option{Meta: ast.Meta{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}, Key: "operating_currency", Value: "EUR"},
		// Transaction commodity must not override explicit option.
		ast.Transaction{
			Meta: ast.Meta{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: &ast.Amount{Number: big.NewRat(1, 1), Commodity: "USD"}},
			},
		},
	}
	got := inferOpCurrency(dirs, nil)
	if got != "EUR" {
		t.Fatalf("got %q want EUR", got)
	}
}

func TestInferOpCurrencyFromFirstTxn(t *testing.T) {
	dirs := []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}, Account: "Assets:Cash"},
		ast.Transaction{
			Meta: ast.Meta{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: &ast.Amount{Number: big.NewRat(10, 1), Commodity: "BRL"}},
				{Account: "Equity:Open", Units: &ast.Amount{Number: big.NewRat(-10, 1), Commodity: "BRL"}},
			},
		},
	}
	got := inferOpCurrency(dirs, &project.Project{})
	if got != "BRL" {
		t.Fatalf("got %q want BRL", got)
	}
}

func TestInferOpCurrencyEmpty(t *testing.T) {
	// Residual-only / no commodities → empty string (caller must tolerate).
	dirs := []ast.Directive{
		ast.Open{Meta: ast.Meta{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}, Account: "Assets:Cash"},
		ast.Transaction{
			Meta: ast.Meta{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
			Postings: []ast.Posting{
				{Account: "Assets:Cash"}, // residual, no units
			},
		},
	}
	if got := inferOpCurrency(dirs, nil); got != "" {
		t.Fatalf("got %q want empty", got)
	}
	if got := inferOpCurrency(nil, nil); got != "" {
		t.Fatalf("nil dirs: got %q", got)
	}
}

func TestDocumentsForAccount(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "example")
	p, pdb, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(p, pdb, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Documents) == 0 {
		t.Fatal("example personal ledger has no documents; fixture changed?")
	}

	// Exact match for an account that has a linked document in the example.
	acct := "Assets:BR:Alfa:ContaCorrente"
	got := l.DocumentsForAccount(acct)
	if len(got) == 0 {
		// Fall back: find any account that has docs and re-check exact filter.
		by := map[string]int{}
		for _, d := range l.Documents {
			by[d.Account]++
		}
		if len(by) == 0 {
			t.Fatal("documents present but all empty account")
		}
		for a := range by {
			acct = a
			got = l.DocumentsForAccount(acct)
			break
		}
	}
	if len(got) == 0 {
		t.Fatalf("DocumentsForAccount(%q) empty; total docs=%d", acct, len(l.Documents))
	}
	for _, d := range got {
		if d.Account != acct {
			t.Fatalf("got account %q want %q", d.Account, acct)
		}
	}

	// Prefix / child must not match (exact name only).
	if child := l.DocumentsForAccount(acct + ":Extra"); len(child) != 0 {
		t.Fatalf("child path should not match, got %d", len(child))
	}
	if none := l.DocumentsForAccount("Assets:Does:Not:Exist"); len(none) != 0 {
		t.Fatalf("missing account: got %d", len(none))
	}
	if empty := l.DocumentsForAccount(""); len(empty) != 0 {
		// Only pass if no document truly has empty account.
		for _, d := range l.Documents {
			if d.Account == "" {
				return
			}
		}
		t.Fatalf("empty account filter: got %d", len(empty))
	}
}
