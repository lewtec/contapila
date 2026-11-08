package config

import (
	"fmt"
	"strings"
	"time"

	"cuelang.org/go/cue"

	"github.com/lucasew/contapila-go/internal/period"
)

// LedgerPlotFrom looks up ledgers.<name>.plot_from.
//
//	ok && err == nil  — valid floor date
//	!ok && err == nil — field absent or empty (no floor)
//	!ok && err != nil — field set but unusable (caller should report check error; no floor)
func LedgerPlotFrom(cfg cue.Value, ledger string) (floor time.Time, ok bool, err error) {
	if !cfg.Exists() || ledger == "" {
		return time.Time{}, false, nil
	}
	v := cfg.LookupPath(cue.MakePath(cue.Str("ledgers"), cue.Str(ledger), cue.Str("plot_from")))
	if !v.Exists() {
		return time.Time{}, false, nil
	}
	s, serr := v.String()
	if serr != nil {
		return time.Time{}, false, fmt.Errorf("ledgers.%s.plot_from: not a string", ledger)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false, nil
	}
	t, perr := period.ParseDate(s)
	if perr != nil || t.IsZero() {
		return time.Time{}, false, fmt.Errorf("ledgers.%s.plot_from: invalid date %q (want YYYY-MM-DD)", ledger, s)
	}
	return period.DateOnly(t), true, nil
}

// NetWorthPlotFrom returns the chart floor when ledgers.<name>.plot_from is valid.
// Invalid values are ignored (false); use LedgerPlotFrom for check diagnostics.
func NetWorthPlotFrom(cfg cue.Value, ledger string) (time.Time, bool) {
	t, ok, err := LedgerPlotFrom(cfg, ledger)
	if err != nil || !ok {
		return time.Time{}, false
	}
	return t, true
}

// ApplyNetWorthPlotFrom floors series start to plotFrom unless the selected
// period lies entirely before plotFrom (explicit pre-audit filter).
// from/to zero mean open bounds (same as period.Range).
func ApplyNetWorthPlotFrom(from, to, plotFrom time.Time) time.Time {
	if plotFrom.IsZero() {
		return from
	}
	plotFrom = period.DateOnly(plotFrom)
	if !to.IsZero() {
		to = period.DateOnly(to)
		if to.Before(plotFrom) {
			return from
		}
	}
	if from.IsZero() {
		return plotFrom
	}
	from = period.DateOnly(from)
	if from.Before(plotFrom) {
		return plotFrom
	}
	return from
}
