package booking

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
	"github.com/lucasew/contapila-go/internal/period"
)

// AutoInterestAccount is an asset open that carries interest_rate metadata.
type AutoInterestAccount struct {
	Account    string
	OpenDate   time.Time
	CloseDate  time.Time // zero if still open
	Currencies []string
	Rate       InterestRate
	Raw        string
	Income     string // Income:Passivo counterpart
	File       string
	Line       int
}

// CollectAutoInterest scans opens/closes for autointerest accounts.
func CollectAutoInterest(dirs []ast.Directive) []AutoInterestAccount {
	byAcct := map[string]*AutoInterestAccount{}
	var order []string
	for _, d := range dirs {
		switch v := d.(type) {
		case ast.Open:
			ir, raw, ok := InterestRateFromMeta(v.Metadata)
			if !ok {
				continue
			}
			a := &AutoInterestAccount{
				Account:    v.Account,
				OpenDate:   v.Date,
				Currencies: append([]string(nil), v.Currencies...),
				Rate:       ir,
				Raw:        raw,
				Income:     IncomePassivoAccount(v.Account),
				File:       v.File,
				Line:       v.Line,
			}
			if _, exists := byAcct[v.Account]; !exists {
				order = append(order, v.Account)
			}
			byAcct[v.Account] = a
		case ast.Close:
			if a := byAcct[v.Account]; a != nil {
				a.CloseDate = v.Date
			}
		}
	}
	out := make([]AutoInterestAccount, 0, len(order))
	for _, name := range order {
		out = append(out, *byAcct[name])
	}
	return out
}

// ExpandAutoInterest inserts:
//   - open Income:Passivo:… for each autointerest asset (if missing)
//   - pad asset←Income day-before each balance on an autointerest asset (if no pad already set)
//   - on close: pad + balance 0 (per open currency) so residual interest zeros before close
//
// Safe to call more than once (idempotent pads). Run again after ExpandClosing so
// synthetic balance 0 / close from closing: TRUE also get autointerest pads.
//
// Projection is not written to the stream (graphs use ProjectedUnits).
func ExpandAutoInterest(dirs []ast.Directive) ([]ast.Directive, diag.List) {
	var diags diag.List
	accounts := CollectAutoInterest(dirs)
	if len(accounts) == 0 {
		return dirs, diags
	}
	ai := map[string]*AutoInterestAccount{}
	for i := range accounts {
		ai[accounts[i].Account] = &accounts[i]
	}

	opened := map[string]bool{}
	for _, d := range dirs {
		if o, ok := d.(ast.Open); ok {
			opened[o.Account] = true
		}
	}

	var synthOpen []ast.Directive
	for _, a := range accounts {
		if opened[a.Income] {
			continue
		}
		opened[a.Income] = true
		synthOpen = append(synthOpen, ast.Open{
			Meta:       ast.Meta{Date: a.OpenDate, File: a.File, Line: a.Line},
			Account:    a.Income,
			Currencies: append([]string(nil), a.Currencies...),
		})
	}

	// Pending pads: account → true if pad seen since last balance on that account.
	userPadPending := map[string]bool{}
	var out []ast.Directive
	out = append(out, dirs...)
	if len(synthOpen) > 0 {
		out = append(synthOpen, out...)
	}

	// Zero-balance already present for account|commodity (avoid double inject on close).
	hasZeroBal := map[string]bool{}
	for _, d := range out {
		if b, ok := d.(ast.Balance); ok && b.Amount.Number != nil && b.Amount.Number.Sign() == 0 {
			hasZeroBal[b.Account+"|"+b.Amount.Commodity] = true
		}
	}

	var final []ast.Directive
	for _, d := range out {
		switch v := d.(type) {
		case ast.Pad:
			userPadPending[v.Account] = true
			final = append(final, d)
		case ast.Balance:
			if a := ai[v.Account]; a != nil {
				if !userPadPending[v.Account] {
					final = append(final, autoInterestPad(a, v.Date, v.File, v.Line))
				}
				userPadPending[v.Account] = false
				if v.Amount.Number != nil && v.Amount.Number.Sign() == 0 {
					hasZeroBal[v.Account+"|"+v.Amount.Commodity] = true
				}
			}
			final = append(final, d)
		case ast.Close:
			if a := ai[v.Account]; a != nil {
				ccys := a.Currencies
				if len(ccys) == 0 {
					// Infer from zero-balances already planned; else cannot pad safely.
					diags.Warn(v.File, v.Line, fmt.Sprintf(
						"autointerest close %s: no currencies on open; skip pad-to-zero (add currency on open)", v.Account))
				} else {
					for _, ccy := range ccys {
						key := v.Account + "|" + ccy
						if hasZeroBal[key] {
							// ExpandClosing (or user) already has balance 0; pad was handled on that Balance.
							continue
						}
						if !userPadPending[v.Account] {
							final = append(final, autoInterestPad(a, v.Date, v.File, v.Line))
							userPadPending[v.Account] = true
						}
						final = append(final, ast.Balance{
							Meta:    ast.Meta{Date: v.Date, File: v.File, Line: v.Line},
							Account: v.Account,
							Amount:  ast.Amount{Number: big.NewRat(0, 1), Commodity: ccy},
						})
						userPadPending[v.Account] = false
						hasZeroBal[key] = true
					}
				}
			}
			final = append(final, d)
		default:
			final = append(final, d)
		}
	}
	return final, diags
}

func autoInterestPad(a *AutoInterestAccount, balDate time.Time, file string, line int) ast.Pad {
	padDate := balDate.AddDate(0, 0, -1)
	if padDate.Before(a.OpenDate) {
		padDate = a.OpenDate
	}
	// Same-day close/balance 0: pad ranks before balance/close in booking sort.
	// Day-before is preferred for ordinary balances; for close-day inject use same day
	// when padDate would still work — keep day-before for balances, same-day when balDate == close.
	return ast.Pad{
		Meta:        ast.Meta{Date: padDate, File: file, Line: line},
		Account:     a.Account,
		FromAccount: a.Income,
	}
}

// ProjectedUnits estimates autointerest account units as-of asOf (inclusive),
// applying daily α*idx+plus on calendar days and treating balance as ground truth.
// Used for graphs; does not mutate the book.
//
// principal is tracked per commodity; only commodities that appear on the account are projected.
func ProjectedUnits(
	account string,
	cfg InterestRate,
	openDate, closeDate time.Time,
	dirs []ast.Directive,
	idx IndexDB,
	asOf time.Time,
) map[string]*big.Rat {
	asOf = period.DateOnly(asOf)
	openDate = period.DateOnly(openDate)
	if !closeDate.IsZero() {
		closeDate = period.DateOnly(closeDate)
		// Projection stops at close (exclusive of growth on/after close day).
		if !asOf.Before(closeDate) {
			asOf = closeDate.AddDate(0, 0, -1)
		}
	}
	if asOf.Before(openDate) {
		return map[string]*big.Rat{}
	}

	bal := map[string]*big.Rat{}
	// Collect dated events for this account.
	type ev struct {
		day   time.Time
		kind  string // txn | balance
		comm  string
		delta *big.Rat // txn delta; balance sets absolute
		set   bool
	}
	var events []ev
	for _, d := range dirs {
		switch v := d.(type) {
		case ast.Transaction:
			for _, p := range v.Postings {
				if p.Account != account || p.Units == nil || p.Units.Number == nil {
					continue
				}
				events = append(events, ev{
					day: period.DateOnly(v.Date), kind: "txn",
					comm: p.Units.Commodity, delta: new(big.Rat).Set(p.Units.Number),
				})
			}
		case ast.Balance:
			if v.Account != account {
				continue
			}
			events = append(events, ev{
				day: period.DateOnly(v.Date), kind: "balance",
				comm: v.Amount.Commodity, delta: new(big.Rat).Set(v.Amount.Number), set: true,
			})
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].day.Equal(events[j].day) {
			return events[i].kind < events[j].kind // balance after txn if same day? "balance" > "txn"
		}
		return events[i].day.Before(events[j].day)
	})

	// Walk day by day from open to asOf.
	// Interest applies at the start of each day after open; then same-day txns/balances.
	// Balance is ground truth (sets absolute units).
	day := openDate
	ei := 0

	applyInterest := func(d time.Time) {
		eff := EffectiveInterestRate(cfg, idx, d)
		if eff.Sign() == 0 {
			return
		}
		for c, b := range bal {
			if b.Sign() == 0 {
				continue
			}
			delta := new(big.Rat).Mul(b, eff)
			bal[c] = new(big.Rat).Add(b, delta)
		}
	}

	for !day.After(asOf) {
		// Interest for this calendar day (using today's index), then events.
		// Align with plugin: interest applies on last_date for each day strictly before next entry;
		// for projection continuous: apply interest every day including days with events *before* events
		// for days after open. Skip interest on open day before any principal? Plugin starts last_updated=open date
		// and accrues while last < entry — first accrual is open day if next entry is later.
		// So on open day itself, if there's a deposit txn same day: process open, then txn updates balance,
		// last_updated = txn.date+1. Interest starts the day after funding in plugin for same-day fund?
		// account_last_updated[account] = entry.date + 1 for txns.
		// For simplicity: apply interest at start of day for days > openDate, then apply same-day events.
		if day.After(openDate) {
			applyInterest(day)
		}
		for ei < len(events) && events[ei].day.Equal(day) {
			e := events[ei]
			ei++
			if e.set {
				bal[e.comm] = new(big.Rat).Set(e.delta)
			} else {
				if bal[e.comm] == nil {
					bal[e.comm] = big.NewRat(0, 1)
				}
				bal[e.comm] = new(big.Rat).Add(bal[e.comm], e.delta)
			}
		}
		// On open day only events (funding), no prior interest.
		day = day.AddDate(0, 0, 1)
	}

	out := map[string]*big.Rat{}
	for c, b := range bal {
		if b.Sign() != 0 {
			out[c] = new(big.Rat).Set(b)
		}
	}
	return out
}

// EffectiveInterestRate returns α*idx + plus for a day (for tests / debug).
func EffectiveInterestRate(cfg InterestRate, idx IndexDB, day time.Time) *big.Rat {
	eff := new(big.Rat).Mul(new(big.Rat).Set(cfg.Alpha), idx.IndexRate(cfg.Indicator, day))
	if cfg.PlusDaily != nil {
		eff.Add(eff, cfg.PlusDaily)
	}
	return eff
}

// ValidateAutoInterestRates warns on unparseable interest_rate meta.
func ValidateAutoInterestRates(dirs []ast.Directive) diag.List {
	var diags diag.List
	for _, d := range dirs {
		o, ok := d.(ast.Open)
		if !ok || len(o.Metadata) == 0 {
			continue
		}
		raw := o.Metadata["interest_rate"]
		if strings.TrimSpace(raw) == "" {
			raw = o.Metadata["interest-rate"]
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if _, ok := ParseInterestRate(raw); !ok {
			diags.Error(o.File, o.Line, fmt.Sprintf("invalid interest_rate %q on %s", raw, o.Account))
		}
	}
	return diags
}
