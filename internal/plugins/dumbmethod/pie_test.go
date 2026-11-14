package dumbmethod

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/web"
)

func TestClassSlicesGeometry(t *testing.T) {
	rows := []web.DumbMethodClassRow{
		{Name: "A", Color: "a"},
		{Name: "B", Color: "b"},
		{Name: "C", Color: "c"},
		{Name: "D", Color: "d"},
	}
	shares := []float64{0.5, 0, 0.3, 0.2}
	got := classSlices(rows, func(i int) float64 { return shares[i] })
	if len(got) != 3 {
		t.Fatalf("slices: %+v", got)
	}
	if got[0].Name != "A" || got[1].Name != "C" || got[2].Name != "D" {
		t.Fatalf("zero share not skipped: %+v", got)
	}
	if got[0].Color != "a" || got[0].Pct != "50.00%" {
		t.Errorf("first slice: %+v", got[0])
	}

	circumference := 2 * math.Pi * DumbMethodDonutRadius
	dash, gap := dashPair(t, got[0].Dash)
	if math.Abs(dash-0.5*circumference) > 0.01 {
		t.Errorf("dash %v want %v", dash, 0.5*circumference)
	}
	if math.Abs(dash+gap-circumference) > 0.01 {
		t.Errorf("dash+gap %v want %v", dash+gap, circumference)
	}

	// Offsets walk the circumference in draw order.
	offsets := []float64{}
	for _, s := range got {
		o, err := strconv.ParseFloat(s.Offset, 64)
		if err != nil {
			t.Fatalf("offset %q: %v", s.Offset, err)
		}
		offsets = append(offsets, o)
	}
	if math.Abs(offsets[0]) > 0.001 {
		t.Errorf("first offset %v want 0", offsets[0])
	}
	if math.Abs(offsets[1]-(-0.5*circumference)) > 0.01 {
		t.Errorf("second offset %v want %v", offsets[1], -0.5*circumference)
	}
	if math.Abs(offsets[2]-(-0.8*circumference)) > 0.01 {
		t.Errorf("third offset %v want %v", offsets[2], -0.8*circumference)
	}
}

func TestClassSlicesFullCircleHasNoGap(t *testing.T) {
	rows := []web.DumbMethodClassRow{{Name: "only", Color: "x"}}
	got := classSlices(rows, func(int) float64 { return 1 })
	if len(got) != 1 {
		t.Fatalf("slices: %+v", got)
	}
	dash, gap := dashPair(t, got[0].Dash)
	if gap != 0 {
		t.Errorf("full circle gap %v want 0 (dash %v)", gap, dash)
	}
}

func dashPair(t *testing.T, s string) (float64, float64) {
	t.Helper()
	parts := strings.Fields(s)
	if len(parts) != 2 {
		t.Fatalf("dasharray %q", s)
	}
	dash, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		t.Fatalf("dash %q: %v", parts[0], err)
	}
	gap, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		t.Fatalf("gap %q: %v", parts[1], err)
	}
	return dash, gap
}
