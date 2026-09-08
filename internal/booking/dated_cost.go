package booking

import (
	"math/big"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/period"
	"github.com/lucasew/contapila-go/internal/prices"
)

// ExpandDatedCosts injects synthetic price directives from cost specs that
// carry an explicit date: {100.00 USD, 2024-03-10} on a HOOL posting becomes
// 2024-03-10 price HOOL 100.00 USD. Also records them on the shared PriceDB
// when provided. @/@@ do not inject prices.
//
// Existing same-day (base,quote) prices in the DB are left alone (no overwrite
// of an explicit price file last-wins entry that was already loaded).
func ExpandDatedCosts(dirs []ast.Directive, pdb *prices.DB) []ast.Directive {
	var synth []ast.Directive
	seen := map[string]bool{} // date|base|quote
	// Pre-index existing price directives in stream.
	for _, d := range dirs {
		if p, ok := d.(ast.Price); ok {
			seen[priceKey(p.Date, p.Currency, p.Amount.Commodity)] = true
		}
	}
	for _, d := range dirs {
		t, ok := d.(ast.Transaction)
		if !ok {
			continue
		}
		for _, p := range t.Postings {
			if p.Cost == nil || p.Cost.Empty || p.Cost.Date.IsZero() {
				continue
			}
			if p.Cost.Number == nil || p.Cost.Commodity == "" {
				continue
			}
			if p.Units == nil || p.Units.Commodity == "" {
				continue
			}
			base := p.Units.Commodity
			quote := p.Cost.Commodity
			dt := period.DateOnly(p.Cost.Date)
			k := priceKey(dt, base, quote)
			if seen[k] {
				continue
			}
			seen[k] = true
			rate := new(big.Rat).Set(p.Cost.Number)
			pr := ast.Price{
				Meta:     ast.Meta{Date: dt, File: t.File, Line: t.Line},
				Currency: base,
				Amount:   ast.Amount{Number: rate, Commodity: quote},
			}
			synth = append(synth, pr)
			if pdb != nil {
				// Last-wins on same day; dated cost contributes a market observation.
				pdb.Add(base, quote, dt, rate)
			}
		}
	}
	if len(synth) == 0 {
		return dirs
	}
	out := make([]ast.Directive, 0, len(dirs)+len(synth))
	out = append(out, dirs...)
	out = append(out, synth...)
	return out
}

func priceKey(d time.Time, base, quote string) string {
	return d.Format("2006-01-02") + "|" + base + "|" + quote
}
