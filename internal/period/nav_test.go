package period

import (
	"testing"
	"time"
)

func TestShiftMonth(t *testing.T) {
	now := d("2024-07-15")
	got, err := Shift("2024-03", now, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2024-04" {
		t.Fatalf("got %q", got)
	}
	got, err = Shift("2024-03", now, -1)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2024-02" {
		t.Fatalf("got %q", got)
	}
	got, err = Shift("", now, 0)
	if err != nil {
		t.Fatal(err)
	}
	// empty + shift 0 via At month of now after defaulting — Shift with delta 0 on empty starts at current month
	if got != "2024-07" {
		// Shift empty with delta 0: sets to current month then shifts 0
		t.Fatalf("empty shift 0: %q", got)
	}
}

func TestSetInterval(t *testing.T) {
	now := d("2024-07-15")
	if g := SetInterval("2024-03", now, KindYear); g != "2024" {
		t.Fatalf("year: %q", g)
	}
	if g := SetInterval("2024-03", now, KindQuarter); g != "2024-Q1" {
		t.Fatalf("quarter: %q", g)
	}
	if g := SetInterval("", now, KindMonth); g != "2024-07" {
		t.Fatalf("month from empty: %q", g)
	}
}

func TestDisplayLabel(t *testing.T) {
	now := d("2024-07-15")
	if g := DisplayLabel("2024-03", now); g != "March 2024" {
		t.Fatalf("got %q", g)
	}
	if g := DisplayLabel("", now); g != "All time" {
		t.Fatalf("got %q", g)
	}
}

func TestKind(t *testing.T) {
	now := time.Now()
	if Kind("2024-03", now) != KindMonth {
		t.Fatal()
	}
	if Kind("month-1", now) != KindMonth {
		t.Fatal()
	}
}
