package config

import (
	"strings"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/period"
)

func TestOperatingCurrency(t *testing.T) {
	cfg, err := Load([]byte("operating_currency: [\"BRL\", \"USD\"]\n"), "t.cue", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := OperatingCurrency(cfg.Value); got != "BRL" {
		t.Fatalf("got %q want BRL", got)
	}
	empty, err := Load([]byte("{}\n"), "t.cue", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := OperatingCurrency(empty.Value); got != "" {
		t.Fatalf("empty want \"\", got %q", got)
	}
}

func TestLedgerPlotFrom(t *testing.T) {
	discovered := []Ledger{
		{Name: "personal", Main: "/proj/personal/main.beancount"},
		{Name: "acme", Main: "/proj/acme/main.beancount"},
	}
	cfg, err := Load([]byte(`
ledgers: {
	personal: {
		plot_from: "2024-06-01"
	}
	acme: {
		plot_from: "not-a-date"
	}
}
`), "t.cue", discovered, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, gerr := LedgerPlotFrom(cfg.Value, "personal")
	if gerr != nil || !ok {
		t.Fatalf("personal: ok=%v err=%v", ok, gerr)
	}
	want := period.DateOnly(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if _, ok := NetWorthPlotFrom(cfg.Value, "acme"); ok {
		t.Fatal("invalid plot_from must not apply as floor")
	}
	_, ok, gerr = LedgerPlotFrom(cfg.Value, "acme")
	if ok || gerr == nil {
		t.Fatalf("acme want invalid err, ok=%v err=%v", ok, gerr)
	}
	if !strings.Contains(gerr.Error(), "plot_from") {
		t.Fatalf("msg: %v", gerr)
	}
	cfg2, err := Load([]byte(`// empty`), "t.cue", discovered, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, gerr := LedgerPlotFrom(cfg2.Value, "personal"); ok || gerr != nil {
		t.Fatalf("absent: ok=%v err=%v", ok, gerr)
	}
}

func TestApplyNetWorthPlotFrom(t *testing.T) {
	floor := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	d := func(y int, m time.Month, day int) time.Time {
		return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
	}
	tests := []struct {
		name     string
		from, to time.Time
		want     time.Time
	}{
		{"all time floors", time.Time{}, time.Time{}, floor},
		{"open start floors", time.Time{}, d(2025, 1, 1), floor},
		{"overlap clamps start", d(2020, 1, 1), d(2025, 1, 1), floor},
		{"after keeps", d(2024, 7, 1), d(2025, 1, 1), d(2024, 7, 1)},
		{"wholly before keeps", d(2020, 1, 1), d(2023, 12, 31), d(2020, 1, 1)},
		{"until before keeps open start", time.Time{}, d(2023, 1, 1), time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyNetWorthPlotFrom(tt.from, tt.to, floor)
			if tt.want.IsZero() {
				if !got.IsZero() {
					t.Fatalf("got %v want zero", got)
				}
				return
			}
			if !period.DateOnly(got).Equal(period.DateOnly(tt.want)) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}
