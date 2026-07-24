package web

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/lucasew/contapila-go/internal/engine"
)

func TestChartBarsJSONEmpty(t *testing.T) {
	js, err := chartBarsJSON(nil, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	if js != "" {
		t.Fatalf("nil bars: want empty JS, got %q", js)
	}
	js, err = chartBarsJSON([]engine.BarPoint{}, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if js != "" {
		t.Fatalf("empty bars: want empty JS, got %q", js)
	}
}

func TestChartBarsJSONPayload(t *testing.T) {
	bars := []engine.BarPoint{
		{Label: "2024-01", Income: big.NewRat(100, 1), Expense: big.NewRat(40, 1)},
		{Label: "2024-02", Income: big.NewRat(50, 1), Expense: nil},
	}
	js, err := chartBarsJSON(bars, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	if js == "" {
		t.Fatal("expected non-empty chart JSON")
	}

	var p struct {
		Kind     string    `json:"kind"`
		Currency string    `json:"currency"`
		X        []float64 `json:"x"`
		Labels   []string  `json:"labels"`
		Income   []float64 `json:"income"`
		Expense  []float64 `json:"expense"`
	}
	if err := json.Unmarshal([]byte(js), &p); err != nil {
		t.Fatalf("unmarshal: %v raw=%s", err, js)
	}
	if p.Kind != "bars" {
		t.Fatalf("kind=%q", p.Kind)
	}
	if p.Currency != "BRL" {
		t.Fatalf("currency=%q", p.Currency)
	}
	if len(p.X) != 2 || p.X[0] != 0 || p.X[1] != 1 {
		t.Fatalf("x ordinals = %v", p.X)
	}
	if len(p.Labels) != 2 || p.Labels[0] != "2024-01" || p.Labels[1] != "2024-02" {
		t.Fatalf("labels=%v", p.Labels)
	}
	if p.Income[0] != 100 || p.Income[1] != 50 {
		t.Fatalf("income=%v", p.Income)
	}
	// nil expense must marshal as 0, not null, so uPlot gets a number series.
	if p.Expense[0] != 40 || p.Expense[1] != 0 {
		t.Fatalf("expense=%v", p.Expense)
	}
}

func TestBuildPnLRows(t *testing.T) {
	lines := []engine.PnLLine{
		{
			Account: "Income", Name: "Income", Depth: 0,
			Amount: big.NewRat(-1500, 1), IsRollup: true, Commodity: "BRL",
		},
		{
			Account: "Income:Salary", Name: "Salary", Depth: 1,
			Amount: big.NewRat(-1500, 1), Commodity: "BRL",
		},
		{
			Account: "Expenses:Food", Name: "", Depth: 1,
			Amount: nil, Commodity: "BRL",
		},
	}
	rows := buildPnLRows(lines)
	if len(rows) != 3 {
		t.Fatalf("rows=%d", len(rows))
	}

	if rows[0].Account != "Income" || rows[0].Name != "Income" || !rows[0].IsRollup {
		t.Fatalf("rollup row: %+v", rows[0])
	}
	if rows[0].Amount != "-1500.00" {
		t.Fatalf("rollup amount=%q want -1500.00 (FloatString 2)", rows[0].Amount)
	}
	if rows[0].PadLeft != "" {
		t.Fatalf("depth 0 pad=%q", rows[0].PadLeft)
	}

	if rows[1].Name != "Salary" || rows[1].Depth != 1 {
		t.Fatalf("leaf: %+v", rows[1])
	}
	if rows[1].PadLeft != "0.75rem" {
		t.Fatalf("depth 1 pad=%q want 0.75rem", rows[1].PadLeft)
	}

	// Empty Name falls back to Account; nil Amount → empty string.
	if rows[2].Name != "Expenses:Food" {
		t.Fatalf("name fallback=%q", rows[2].Name)
	}
	if rows[2].Amount != "" {
		t.Fatalf("nil amount should format empty, got %q", rows[2].Amount)
	}
}

func TestBuildNetWorthRows(t *testing.T) {
	lines := []engine.NetWorthTreeLine{
		{
			Account: "Assets", Path: "Assets", Name: "Assets", Depth: 0,
			Units: nil, Commodity: "", Value: big.NewRat(1000, 1), IsRollup: true,
		},
		{
			Account: "Assets:Cash", Path: "Assets:Cash\x1fBRL", Name: "Cash", Depth: 1,
			Units: big.NewRat(1000, 1), Commodity: "BRL", Value: big.NewRat(1000, 1),
		},
		{
			Account: "Assets:Stock", Path: "", Name: "Stock", Depth: 1,
			Units: big.NewRat(10, 1), Commodity: "XYZ", Value: nil, Unpriced: true,
		},
	}
	rows := buildNetWorthRows(lines)
	if len(rows) != 3 {
		t.Fatalf("rows=%d", len(rows))
	}

	if !rows[0].IsRollup || rows[0].Units != "" {
		t.Fatalf("parent: %+v", rows[0])
	}
	if rows[0].Value != "1000.00" {
		t.Fatalf("parent value=%q", rows[0].Value)
	}
	if rows[0].Path != "Assets" {
		t.Fatalf("path=%q", rows[0].Path)
	}

	if rows[1].Units != "1000.0000" {
		t.Fatalf("units=%q want FloatString 4", rows[1].Units)
	}
	if rows[1].Value != "1000.00" {
		t.Fatalf("value=%q", rows[1].Value)
	}
	if rows[1].PadLeft != "0.75rem" {
		t.Fatalf("pad=%q", rows[1].PadLeft)
	}

	// Empty Path falls back to Account; nil Value → ""; Unpriced preserved.
	if rows[2].Path != "Assets:Stock" {
		t.Fatalf("path fallback=%q", rows[2].Path)
	}
	if rows[2].Value != "" {
		t.Fatalf("nil value should be empty, got %q", rows[2].Value)
	}
	if !rows[2].Unpriced {
		t.Fatal("expected Unpriced")
	}
}
