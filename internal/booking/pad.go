package booking

import (
	"math/big"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
	"github.com/lucasew/contapila-go/internal/period"
)

// ExpandPads materializes Beancount pad directives as synthetic transactions
// dated at the pad directive, not at the later balance assertion that computes
// the difference. This keeps historical reports/series aligned with Beancount.
func ExpandPads(dirs []ast.Directive, setup func(*Engine)) ([]ast.Directive, diag.List) {
	var diags diag.List
	probe := New()
	if setup != nil {
		setup(probe)
	}

	var synth []ast.Directive
	for _, d := range sortedDirectives(dirs) {
		if b, ok := d.(ast.Balance); ok {
			actual := probe.balOf(b.Account, b.Amount.Commodity)
			diff := new(big.Rat).Sub(new(big.Rat).Set(b.Amount.Number), actual)
			if new(big.Rat).Abs(diff).Cmp(probe.tol(b.Amount.Commodity)) > 0 {
				if pad, ok := probe.Pad[b.Account]; ok {
					if !period.DateOnly(pad.Date).Equal(period.DateOnly(b.Date)) {
						synth = append(synth, padTxn(pad, b, diff))
					}
				}
			}
		}
		probe.Book([]ast.Directive{d})
	}

	if len(synth) == 0 {
		return dirs, diags
	}
	out := make([]ast.Directive, 0, len(dirs)+len(synth))
	out = append(out, dirs...)
	out = append(out, synth...)
	return out, diags
}

func padTxn(p ast.Pad, b ast.Balance, diff *big.Rat) ast.Transaction {
	n := new(big.Rat).Set(diff)
	return ast.Transaction{
		Meta:      ast.Meta{Date: p.Date, File: p.File, Line: p.Line},
		Flag:      "P",
		Narration: "pad",
		Postings: []ast.Posting{
			{Account: b.Account, Units: &ast.Amount{Number: n, Commodity: b.Amount.Commodity}},
			{Account: p.FromAccount, Units: &ast.Amount{Number: new(big.Rat).Neg(n), Commodity: b.Amount.Commodity}},
		},
	}
}
