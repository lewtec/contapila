package engine

import (
	"math/big"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
)

func d(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", s, time.UTC)
	return t
}
func r(s string) *big.Rat {
	x, _ := new(big.Rat).SetString(s)
	return x
}

// Beancount: liability residual is credit (negative units). NW = cash + loan.
func TestNetWorthLiabilitySignConvention(t *testing.T) {
	e := booking.New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Liabilities:Loan"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Income:Sales"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-02-01")}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: &ast.Amount{Number: r("1000"), Commodity: "BRL"}},
				{Account: "Income:Sales"},
			},
		},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-03-01")}, Flag: "*",
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: &ast.Amount{Number: r("200"), Commodity: "BRL"}},
				{Account: "Liabilities:Loan"},
			},
		},
	})
	cash := e.AllBalances()["Assets:Cash"]["BRL"]
	loan := e.AllBalances()["Liabilities:Loan"]["BRL"]
	if cash.Cmp(r("1200")) != 0 {
		t.Fatalf("cash %s", cash.FloatString(2))
	}
	if loan.Sign() >= 0 {
		t.Fatalf("expected credit liability (negative), got %s", loan.FloatString(2))
	}
	// Natural NW: 1200 + (-200) = 1000 (loan reduces NW, already via sign)
	natural := new(big.Rat).Add(cash, loan)
	if natural.Cmp(r("1000")) != 0 {
		t.Fatalf("natural NW %s want 1000", natural.FloatString(2))
	}
}
