package booking

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
	"github.com/lucasew/contapila-go/internal/period"
)

// ExpandClosing turns posting metadata closing: TRUE into synthetic directives:
//
//	next day: balance Account  0 COMMODITY   (only that posting's commodity)
//	next day: close Account
//
// Commodity/units come from booked (filled) postings so residual legs reuse the
// same inference as bookTxn. Call after a probe Book; pass e.Txns.
//
// Rules:
//   - filled posting with no commodity (empty residual / unbooked) → error
//   - user already wrote close for the account → warn, skip synthetic close (balance 0 still injected)
//   - multi-commodity accounts: only balance-zero the commodity on the marked posting
//
// Synthetic directives keep the txn's File/Line for diagnostics. Appended after dirs
// (booking sorts by date; same-day order is stable).
func ExpandClosing(dirs []ast.Directive, booked []BookedTxn) ([]ast.Directive, diag.List) {
	var diags diag.List
	userClose := map[string]bool{}
	for _, d := range dirs {
		if c, ok := d.(ast.Close); ok && c.Account != "" {
			userClose[c.Account] = true
		}
	}

	var synth []ast.Directive
	plannedClose := map[string]bool{}
	plannedBal := map[string]bool{} // account|commodity
	// file|line|account → handled via booked filled postings
	handled := map[string]bool{}

	for _, bt := range booked {
		t := bt.Txn
		for _, fp := range bt.Postings {
			if !metaTruthy(fp.Metadata, "closing") {
				continue
			}
			hk := closingKey(t.File, t.Line, fp.Account)
			if fp.Units == nil || fp.Units.Commodity == "" {
				diags.Error(t.File, t.Line, fmt.Sprintf("closing: TRUE on %s: could not infer commodity (empty residual?)", fp.Account))
				handled[hk] = true // don't double-report as unbooked
				continue
			}
			handled[hk] = true
			next := nextDay(t.Date)
			balKey := fp.Account + "|" + fp.Units.Commodity
			if !plannedBal[balKey] {
				plannedBal[balKey] = true
				synth = append(synth, ast.Balance{
					Meta:    ast.Meta{Date: next, File: t.File, Line: t.Line},
					Account: fp.Account,
					Amount:  ast.Amount{Number: big.NewRat(0, 1), Commodity: fp.Units.Commodity},
				})
			}
			if userClose[fp.Account] {
				diags.Warn(t.File, t.Line, fmt.Sprintf("closing: TRUE but close already written for %s", fp.Account))
				continue
			}
			if plannedClose[fp.Account] {
				continue
			}
			plannedClose[fp.Account] = true
			synth = append(synth, ast.Close{
				Meta:    ast.Meta{Date: next, File: t.File, Line: t.Line},
				Account: fp.Account,
			})
		}
	}

	// closing: TRUE on postings that never appeared in booked results (txn failed to book).
	for _, d := range dirs {
		t, ok := d.(ast.Transaction)
		if !ok {
			continue
		}
		for _, p := range t.Postings {
			if !metaTruthy(p.Metadata, "closing") {
				continue
			}
			hk := closingKey(t.File, t.Line, p.Account)
			if handled[hk] {
				continue
			}
			diags.Error(t.File, t.Line, fmt.Sprintf("closing: TRUE on %s: posting not booked (cannot infer residual units)", p.Account))
		}
	}

	if len(synth) == 0 {
		return dirs, diags
	}
	out := make([]ast.Directive, 0, len(dirs)+len(synth))
	out = append(out, dirs...)
	out = append(out, synth...)
	return out, diags
}

// BookWithClosing books dirs, expands closing: TRUE from filled postings (including
// residuals), re-runs ExpandAutoInterest so synthetic balance 0 / close get interest
// pads, then re-books with synthetic balance/close so assertions run in date order.
// When no closing metadata is present, books once.
// setup is optional (e.g. set CommTol); applied to each Engine before Book.
func BookWithClosing(dirs []ast.Directive, setup func(*Engine)) (e *Engine, out []ast.Directive, diags diag.List) {
	out = dirs
	newE := func() *Engine {
		eng := New()
		if setup != nil {
			setup(eng)
		}
		return eng
	}
	if !hasClosingMeta(dirs) {
		e = newE()
		e.Book(dirs)
		return e, dirs, e.Diags
	}
	probe := newE()
	probe.Book(dirs)
	var cdiags diag.List
	out, cdiags = ExpandClosing(dirs, probe.Txns)
	diags.Merge(cdiags)
	// After autoclose, pad autointerest accounts into synthetic balance 0 / close.
	var adiags diag.List
	out, adiags = ExpandAutoInterest(out)
	diags.Merge(adiags)
	e = newE()
	e.Book(out)
	diags.Merge(e.Diags)
	return e, out, diags
}

func hasClosingMeta(dirs []ast.Directive) bool {
	for _, d := range dirs {
		t, ok := d.(ast.Transaction)
		if !ok {
			continue
		}
		for _, p := range t.Postings {
			if metaTruthy(p.Metadata, "closing") {
				return true
			}
		}
	}
	return false
}

func closingKey(file string, line int, account string) string {
	return fmt.Sprintf("%s\x00%d\x00%s", file, line, account)
}

func metaTruthy(md ast.Metadata, key string) bool {
	if len(md) == 0 {
		return false
	}
	v, ok := md[key]
	if !ok {
		return false
	}
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

func nextDay(t time.Time) time.Time {
	// Calendar day in UTC (directive dates are date-only UTC).
	return period.DateOnly(t).AddDate(0, 0, 1)
}
