package period

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", s, time.UTC)
	return t
}

func TestChartBinFromFilter(t *testing.T) {
	cases := []struct {
		raw  string
		r    Range
		want BinKind
	}{
		{"2024", Range{Start: day("2024-01-01"), End: day("2024-12-31")}, BinMonth},
		{"2024-03", Range{Start: day("2024-03-01"), End: day("2024-03-31")}, BinDay},
		{"2024-Q1", Range{Start: day("2024-01-01"), End: day("2024-03-31")}, BinMonth},
		{"2020 - 2024", Range{Start: day("2020-01-01"), End: day("2024-12-31")}, BinYear},
		{"", Range{}, BinMonth},
	}
	for _, c := range cases {
		got := ChartBin(c.raw, c.r)
		if got != c.want {
			t.Errorf("ChartBin(%q)=%s want %s", c.raw, got, c.want)
		}
	}
}

func TestIterateBinsMonth(t *testing.T) {
	r := Range{Start: day("2024-01-15"), End: day("2024-03-10")}
	bins := IterateBins(r, BinMonth)
	if len(bins) != 3 {
		t.Fatalf("bins=%d %+v", len(bins), bins)
	}
	if bins[0].Start != day("2024-01-15") || bins[0].End != day("2024-01-31") {
		t.Fatalf("first=%+v", bins[0])
	}
	if bins[2].End != day("2024-03-10") {
		t.Fatalf("last=%+v", bins[2])
	}
}
