package engine

import (
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/period"
)

func TestPnLBarsArePerBinFlows(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "example")
	p, pdb, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(p, pdb, "personal")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	bars := l.PnLBars(from, to, period.BinMonth)
	if len(bars) != 12 {
		t.Fatalf("bars=%d", len(bars))
	}
	// Not a running total: Jan != Jan+Feb
	if bars[1].Income.Cmp(new(big.Rat).Add(bars[0].Income, bars[1].Income)) == 0 && bars[0].Income.Sign() != 0 {
		t.Fatal("looks cumulative")
	}
	// Feb should not equal full year
	sum := big.NewRat(0, 1)
	for _, b := range bars {
		sum.Add(sum, b.Income)
	}
	if bars[0].Income.Cmp(sum) == 0 && sum.Sign() != 0 {
		t.Fatal("first bar equals year sum — cumulative bug")
	}
}

func TestPnLBarsTrimEmptyEdges(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "example")
	p, pdb, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(p, pdb, "personal")
	if err != nil {
		t.Fatal(err)
	}
	// Full 2026 calendar year, but ledger activity stops mid-year → no empty months after last event.
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	bars := l.PnLBars(from, to, period.BinMonth)
	if len(bars) == 0 {
		t.Fatal("expected some 2026 activity")
	}
	if len(bars) == 12 {
		t.Fatal("expected trailing empty months trimmed, got full year")
	}
	if barEmpty(bars[0]) || barEmpty(bars[len(bars)-1]) {
		t.Fatalf("edges still empty: first=%v last=%v", bars[0], bars[len(bars)-1])
	}
	// last label should not be 2026-12 if July is last with data in example
	last := bars[len(bars)-1].Label
	if last == "2026-12" {
		t.Fatalf("trailing empty not trimmed, last=%s", last)
	}
	t.Logf("kept %d bins, %s … %s", len(bars), bars[0].Label, last)
}
