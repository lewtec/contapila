package period

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := ParseDate(s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseDate(t *testing.T) {
	got, err := ParseDate("")
	if err != nil || !got.IsZero() {
		t.Fatalf("empty: got %v %v", got, err)
	}
	got, err = ParseDate("2024-07-15")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2024, 7, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if _, err = ParseDate("not-a-date"); err == nil {
		t.Fatal("expected error for invalid date")
	}
}

func TestParseYearMonth(t *testing.T) {
	now := d("2024-07-15")
	r, err := Parse("2024", now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Start.Equal(d("2024-01-01")) || !r.End.Equal(d("2024-12-31")) {
		t.Fatalf("year: %v %v", r.Start, r.End)
	}
	r, err = Parse("2024-03", now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Start.Equal(d("2024-03-01")) || !r.End.Equal(d("2024-03-31")) {
		t.Fatalf("month: %v %v", r.Start, r.End)
	}
	r, err = Parse("2024-Q2", now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Start.Equal(d("2024-04-01")) || !r.End.Equal(d("2024-06-30")) {
		t.Fatalf("quarter: %v %v", r.Start, r.End)
	}
}

func TestParseRelativeAndRange(t *testing.T) {
	now := d("2024-07-15")
	r, err := Parse("month", now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Start.Equal(d("2024-07-01")) || !r.End.Equal(d("2024-07-31")) {
		t.Fatalf("month: %v %v", r.Start, r.End)
	}
	r, err = Parse("month-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Start.Equal(d("2024-06-01")) || !r.End.Equal(d("2024-06-30")) {
		t.Fatalf("month-1: %v %v", r.Start, r.End)
	}
	r, err = Parse("year-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Start.Equal(d("2023-01-01")) || !r.End.Equal(d("2023-12-31")) {
		t.Fatalf("year-1: %v %v", r.Start, r.End)
	}
	r, err = Parse("2023 - 2024-03", now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Start.Equal(d("2023-01-01")) || !r.End.Equal(d("2024-03-31")) {
		t.Fatalf("range: %v %v", r.Start, r.End)
	}
}

func TestParseDayWeek(t *testing.T) {
	now := d("2024-07-15")
	r, err := Parse("2024-07-15", now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Start.Equal(d("2024-07-15")) || !r.End.Equal(d("2024-07-15")) {
		t.Fatalf("day: %v %v", r.Start, r.End)
	}
	r, err = Parse("2024-W29", now)
	if err != nil {
		t.Fatal(err)
	}
	// ISO week 29 of 2024 starts Monday 2024-07-15
	if !r.Start.Equal(d("2024-07-15")) || !r.End.Equal(d("2024-07-21")) {
		t.Fatalf("week: %v %v", r.Start, r.End)
	}
}
