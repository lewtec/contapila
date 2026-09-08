package engine

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/period"
)

func exampleRoot() string {
	return filepath.Join("..", "..", "testdata", "example")
}

func mustOpenExamplePersonal(b *testing.B) *Ledger {
	b.Helper()
	p, pdb, _, err := OpenProject(exampleRoot())
	if err != nil {
		b.Fatal(err)
	}
	l, err := OpenLedger(p, pdb, "personal")
	if err != nil {
		b.Fatal(err)
	}
	return l
}

// BenchmarkOpenLedger_examplePersonal times loading+booking the dogfood personal
// ledger. Project+prices stay outside the timed loop; ExpandDatedCosts may write
// into the shared PriceDB, but keys are deterministic so later iters stay stable.
func BenchmarkOpenLedger_examplePersonal(b *testing.B) {
	p, pdb, _, err := OpenProject(exampleRoot())
	if err != nil {
		b.Fatal(err)
	}
	// Sanity once so fixture failures fail fast outside the loop.
	if _, err := OpenLedger(p, pdb, "personal"); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		l, err := OpenLedger(p, pdb, "personal")
		if err != nil {
			b.Fatal(err)
		}
		if l.OpCurrency == "" {
			b.Fatal("expected op currency")
		}
	}
}

func BenchmarkNetWorthSeries_examplePersonal(b *testing.B) {
	l := mustOpenExamplePersonal(b)
	// Full open range matches chart/default CLI paths.
	from, to := time.Time{}, time.Time{}

	pts, err := l.NetWorthSeries(from, to)
	if err != nil {
		b.Fatal(err)
	}
	if len(pts) == 0 {
		b.Fatal("expected net-worth points")
	}

	b.ReportAllocs()
	for b.Loop() {
		pts, err := l.NetWorthSeries(from, to)
		if err != nil {
			b.Fatal(err)
		}
		if len(pts) == 0 {
			b.Fatal("expected net-worth points")
		}
	}
}

func BenchmarkAccountSeries_examplePersonal(b *testing.B) {
	l := mustOpenExamplePersonal(b)
	const acct = "Assets:BR:Alfa:ContaCorrente"
	from, to := time.Time{}, time.Time{}

	pts, err := l.AccountSeries(acct, from, to)
	if err != nil {
		b.Fatal(err)
	}
	if len(pts) == 0 {
		b.Fatal("expected account series points")
	}

	b.ReportAllocs()
	for b.Loop() {
		pts, err := l.AccountSeries(acct, from, to)
		if err != nil {
			b.Fatal(err)
		}
		if len(pts) == 0 {
			b.Fatal("expected account series points")
		}
	}
}

func BenchmarkBalancesAsOf_examplePersonal(b *testing.B) {
	l := mustOpenExamplePersonal(b)
	asOf := AsOfLatest

	if bals := l.BalancesAsOf(asOf); len(bals) == 0 {
		b.Fatal("expected balances")
	}

	b.ReportAllocs()
	for b.Loop() {
		if bals := l.BalancesAsOf(asOf); len(bals) == 0 {
			b.Fatal("expected balances")
		}
	}
}

func BenchmarkPnLBars_examplePersonal(b *testing.B) {
	l := mustOpenExamplePersonal(b)
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	if bars := l.PnLBars(from, to, period.BinMonth); len(bars) == 0 {
		b.Fatal("expected pnl bars")
	}

	b.ReportAllocs()
	for b.Loop() {
		if bars := l.PnLBars(from, to, period.BinMonth); len(bars) == 0 {
			b.Fatal("expected pnl bars")
		}
	}
}
