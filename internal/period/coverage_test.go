package period

import (
	"testing"
	"time"
)

func TestRangeEmpty(t *testing.T) {
	cases := []struct {
		name string
		r    Range
		want bool
	}{
		{"zero", Range{}, true},
		{"start only", Range{Start: d("2024-01-01")}, false},
		{"end only", Range{End: d("2024-12-31")}, false},
		{"both", Range{Start: d("2024-01-01"), End: d("2024-12-31")}, false},
		{"raw only still empty", Range{Raw: "all"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.r.Empty(); got != tc.want {
				t.Fatalf("Empty()=%v want %v", got, tc.want)
			}
		})
	}
}

func TestRangeLabel(t *testing.T) {
	cases := []struct {
		name string
		r    Range
		want string
	}{
		{"raw wins", Range{Raw: "month-1", Start: d("2024-06-01"), End: d("2024-06-30")}, "month-1"},
		{"all time", Range{}, "all time"},
		{"same day", Range{Start: d("2024-07-15"), End: d("2024-07-15")}, "2024-07-15"},
		{"closed span", Range{Start: d("2024-01-01"), End: d("2024-03-31")}, "2024-01-01 … 2024-03-31"},
		{"from only", Range{Start: d("2024-01-01")}, "from 2024-01-01"},
		{"until only", Range{End: d("2024-12-31")}, "until 2024-12-31"},
		// same calendar day, different clock times still collapse via sameDay
		{
			"same day different clock",
			Range{
				Start: time.Date(2024, 7, 15, 9, 0, 0, 0, time.UTC),
				End:   time.Date(2024, 7, 15, 18, 0, 0, 0, time.UTC),
			},
			"2024-07-15",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.r.Label(); got != tc.want {
				t.Fatalf("Label()=%q want %q", got, tc.want)
			}
		})
	}
}

func TestBinKindString(t *testing.T) {
	cases := []struct {
		k    BinKind
		want string
	}{
		{BinDay, "day"},
		{BinWeek, "week"},
		{BinMonth, "month"},
		{BinYear, "year"},
		{BinKind(99), "month"}, // unknown falls through to month
	}
	for _, tc := range cases {
		if got := tc.k.String(); got != tc.want {
			t.Errorf("%v.String()=%q want %q", tc.k, got, tc.want)
		}
	}
}

func TestBinFromSpan(t *testing.T) {
	cases := []struct {
		name string
		r    Range
		want BinKind
	}{
		{"open both", Range{}, BinMonth},
		{"open start", Range{End: d("2024-12-31")}, BinMonth},
		{"open end", Range{Start: d("2024-01-01")}, BinMonth},
		{"multi year", Range{Start: d("2020-01-01"), End: d("2022-06-01")}, BinYear},
		{"exactly two year gap", Range{Start: d("2020-06-01"), End: d("2022-01-01")}, BinYear},
		{"adjacent years not multi", Range{Start: d("2023-01-01"), End: d("2024-12-31")}, BinMonth},
		{"single day", Range{Start: d("2024-07-15"), End: d("2024-07-15")}, BinDay},
		{"under two months", Range{Start: d("2024-01-01"), End: d("2024-02-15")}, BinDay},
		{"exactly two months boundary day", Range{Start: d("2024-01-01"), End: d("2024-03-01")}, BinMonth},
		{"long single year", Range{Start: d("2024-01-01"), End: d("2024-12-31")}, BinMonth},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := binFromSpan(tc.r); got != tc.want {
				t.Fatalf("binFromSpan=%s want %s", got, tc.want)
			}
		})
	}
}

func TestChartBinEdges(t *testing.T) {
	cases := []struct {
		raw  string
		r    Range
		want BinKind
	}{
		// token rules
		{"2024-07-15", Range{Start: d("2024-07-15"), End: d("2024-07-15")}, BinDay},
		{"day", Range{}, BinDay},
		{"day-1", Range{}, BinDay},
		{"2024-W12", Range{}, BinDay},
		{"week", Range{}, BinDay},
		{"week+1", Range{}, BinDay},
		{"2024-03", Range{}, BinDay},
		{"month-1", Range{}, BinDay},
		{"2024-Q1", Range{}, BinMonth},
		{"quarter", Range{}, BinMonth},
		{"2024", Range{}, BinMonth},
		{"year-1", Range{}, BinMonth},
		// explicit range → span
		{"2020 - 2024", Range{Start: d("2020-01-01"), End: d("2024-12-31")}, BinYear},
		{"2024-01 - 2024-02", Range{Start: d("2024-01-01"), End: d("2024-02-29")}, BinDay},
		// empty / unknown → span
		{"", Range{Start: d("2024-01-01"), End: d("2024-01-01")}, BinDay},
		{"  ", Range{}, BinMonth},
		{"not-a-filter", Range{Start: d("2020-01-01"), End: d("2023-01-01")}, BinYear},
	}
	for _, tc := range cases {
		name := tc.raw
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			got := ChartBin(tc.raw, tc.r)
			if got != tc.want {
				t.Fatalf("ChartBin(%q)=%s want %s", tc.raw, got, tc.want)
			}
		})
	}
}

func TestIterateBinsEdges(t *testing.T) {
	t.Run("open ends nil", func(t *testing.T) {
		if got := IterateBins(Range{Start: d("2024-01-01")}, BinMonth); got != nil {
			t.Fatalf("open end: %v", got)
		}
		if got := IterateBins(Range{End: d("2024-01-01")}, BinDay); got != nil {
			t.Fatalf("open start: %v", got)
		}
		if got := IterateBins(Range{}, BinYear); got != nil {
			t.Fatalf("both open: %v", got)
		}
	})

	t.Run("start after end", func(t *testing.T) {
		r := Range{Start: d("2024-03-01"), End: d("2024-01-01")}
		if got := IterateBins(r, BinMonth); got != nil {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("day bins", func(t *testing.T) {
		r := Range{Start: d("2024-07-15"), End: d("2024-07-17")}
		bins := IterateBins(r, BinDay)
		if len(bins) != 3 {
			t.Fatalf("len=%d %+v", len(bins), bins)
		}
		if !bins[0].Start.Equal(d("2024-07-15")) || !bins[2].End.Equal(d("2024-07-17")) {
			t.Fatalf("%+v", bins)
		}
	})

	t.Run("week bins clipped", func(t *testing.T) {
		// Wed 2024-07-17 is ISO week containing Mon 2024-07-15
		r := Range{Start: d("2024-07-17"), End: d("2024-07-25")}
		bins := IterateBins(r, BinWeek)
		if len(bins) != 2 {
			t.Fatalf("len=%d %+v", len(bins), bins)
		}
		// first bin clipped to range start (not Monday)
		if !bins[0].Start.Equal(d("2024-07-17")) {
			t.Fatalf("first start=%v", bins[0].Start)
		}
		// week of 2024-07-15 ends Sunday 2024-07-21
		if !bins[0].End.Equal(d("2024-07-21")) {
			t.Fatalf("first end=%v", bins[0].End)
		}
		// second week Mon 2024-07-22 … clipped to 2024-07-25
		if !bins[1].Start.Equal(d("2024-07-22")) || !bins[1].End.Equal(d("2024-07-25")) {
			t.Fatalf("second=%+v", bins[1])
		}
	})

	t.Run("month bins", func(t *testing.T) {
		r := Range{Start: d("2024-01-15"), End: d("2024-03-10")}
		bins := IterateBins(r, BinMonth)
		if len(bins) != 3 {
			t.Fatalf("len=%d", len(bins))
		}
		if !bins[0].Start.Equal(d("2024-01-15")) || !bins[0].End.Equal(d("2024-01-31")) {
			t.Fatalf("jan=%+v", bins[0])
		}
		if !bins[1].Start.Equal(d("2024-02-01")) || !bins[1].End.Equal(d("2024-02-29")) {
			t.Fatalf("feb leap=%+v", bins[1])
		}
		if !bins[2].End.Equal(d("2024-03-10")) {
			t.Fatalf("mar=%+v", bins[2])
		}
	})

	t.Run("year bins", func(t *testing.T) {
		r := Range{Start: d("2022-06-01"), End: d("2024-03-15")}
		bins := IterateBins(r, BinYear)
		if len(bins) != 3 {
			t.Fatalf("len=%d %+v", len(bins), bins)
		}
		if !bins[0].Start.Equal(d("2022-06-01")) || !bins[0].End.Equal(d("2022-12-31")) {
			t.Fatalf("2022=%+v", bins[0])
		}
		if !bins[1].Start.Equal(d("2023-01-01")) || !bins[1].End.Equal(d("2023-12-31")) {
			t.Fatalf("2023=%+v", bins[1])
		}
		if !bins[2].Start.Equal(d("2024-01-01")) || !bins[2].End.Equal(d("2024-03-15")) {
			t.Fatalf("2024=%+v", bins[2])
		}
	})

	t.Run("single day all kinds", func(t *testing.T) {
		r := Range{Start: d("2024-07-15"), End: d("2024-07-15")}
		for _, kind := range []BinKind{BinDay, BinWeek, BinMonth, BinYear} {
			bins := IterateBins(r, kind)
			if len(bins) != 1 {
				t.Fatalf("%s: len=%d", kind, len(bins))
			}
			if !bins[0].Start.Equal(d("2024-07-15")) || !bins[0].End.Equal(d("2024-07-15")) {
				t.Fatalf("%s: %+v", kind, bins[0])
			}
		}
	})
}

func TestStartOfBinNextBin(t *testing.T) {
	// Sunday → Monday of same ISO week (2024-07-14 is Sunday → 2024-07-08)
	sun := d("2024-07-14")
	if got := startOfBin(sun, BinWeek); !got.Equal(d("2024-07-08")) {
		t.Fatalf("week from Sunday: %v", got)
	}
	// Monday stays
	mon := d("2024-07-15")
	if got := startOfBin(mon, BinWeek); !got.Equal(mon) {
		t.Fatalf("week from Monday: %v", got)
	}
	if got := startOfBin(d("2024-07-15"), BinDay); !got.Equal(d("2024-07-15")) {
		t.Fatalf("day: %v", got)
	}
	if got := startOfBin(d("2024-07-15"), BinMonth); !got.Equal(d("2024-07-01")) {
		t.Fatalf("month: %v", got)
	}
	if got := startOfBin(d("2024-07-15"), BinYear); !got.Equal(d("2024-01-01")) {
		t.Fatalf("year: %v", got)
	}

	if got := nextBin(d("2024-07-15"), BinDay); !got.Equal(d("2024-07-16")) {
		t.Fatalf("next day: %v", got)
	}
	if got := nextBin(d("2024-07-15"), BinWeek); !got.Equal(d("2024-07-22")) {
		t.Fatalf("next week: %v", got)
	}
	if got := nextBin(d("2024-01-01"), BinMonth); !got.Equal(d("2024-02-01")) {
		t.Fatalf("next month: %v", got)
	}
	// Dec → Jan next year
	if got := nextBin(d("2024-12-01"), BinMonth); !got.Equal(d("2025-01-01")) {
		t.Fatalf("month year roll: %v", got)
	}
	if got := nextBin(d("2024-01-01"), BinYear); !got.Equal(d("2025-01-01")) {
		t.Fatalf("next year: %v", got)
	}
}

func TestShiftEdgesAndInvalid(t *testing.T) {
	now := d("2024-07-15")

	cases := []struct {
		name   string
		filter string
		delta  int
		want   string
		err    bool
	}{
		// month year boundary
		{"dec to jan", "2024-12", 1, "2025-01", false},
		{"jan to dec prev", "2024-01", -1, "2023-12", false},
		// year edges
		{"year +1", "2024", 1, "2025", false},
		{"year -1", "2024", -1, "2023", false},
		// quarter across year
		{"q4 to q1", "2024-Q4", 1, "2025-Q1", false},
		{"q1 to q4 prev", "2024-Q1", -1, "2023-Q4", false},
		// week / day
		{"week +1", "2024-W29", 1, "2024-W30", false},
		{"day +1", "2024-07-15", 1, "2024-07-16", false},
		{"day year roll", "2024-12-31", 1, "2025-01-01", false},
		// relative resolved then shifted
		{"rel month +1", "month", 1, "2024-08", false},
		{"rel year -1", "year", -1, "2023", false},
		// custom closed range: shift by months, keep day span
		{"custom range", "2024-01-01 - 2024-01-31", 1, "2024-02-01 - 2024-03-02", false},
		// empty defaults to current month
		{"empty +1", "", 1, "2024-08", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Shift(tc.filter, now, tc.delta)
			if tc.err {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}

	// invalid: cannot shift open-ended custom range (end-only is not expressible via Parse easily;
	// use a custom that parses but has zero start — empty filter is not custom).
	// Unrecognized filter becomes KindCustom and fails Parse.
	if _, err := Shift("not-a-period", now, 1); err == nil {
		t.Fatal("expected error for unrecognized filter")
	}
	// start-after-end range is invalid to parse
	if _, err := Shift("2024-06 - 2024-01", now, 1); err == nil {
		t.Fatal("expected error for inverted range")
	}
}

func TestParseRelativeOffsets(t *testing.T) {
	now := d("2024-07-15") // Monday, ISO week 29, Q3

	cases := []struct {
		filter    string
		wantStart string
		wantEnd   string
	}{
		{"day", "2024-07-15", "2024-07-15"},
		{"day-1", "2024-07-14", "2024-07-14"},
		{"day+1", "2024-07-16", "2024-07-16"},
		{"week", "2024-07-15", "2024-07-21"},
		{"week-1", "2024-07-08", "2024-07-14"},
		{"week+1", "2024-07-22", "2024-07-28"},
		{"month", "2024-07-01", "2024-07-31"},
		{"month-1", "2024-06-01", "2024-06-30"},
		{"month+1", "2024-08-01", "2024-08-31"},
		{"quarter", "2024-07-01", "2024-09-30"},
		{"quarter-1", "2024-04-01", "2024-06-30"},
		{"quarter+1", "2024-10-01", "2024-12-31"},
		{"year", "2024-01-01", "2024-12-31"},
		{"year-1", "2023-01-01", "2023-12-31"},
		{"year+1", "2025-01-01", "2025-12-31"},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			r, err := Parse(tc.filter, now)
			if err != nil {
				t.Fatal(err)
			}
			if !r.Start.Equal(d(tc.wantStart)) || !r.End.Equal(d(tc.wantEnd)) {
				t.Fatalf("got %v…%v want %s…%s", r.Start, r.End, tc.wantStart, tc.wantEnd)
			}
			if r.Raw != tc.filter {
				t.Fatalf("Raw=%q", r.Raw)
			}
		})
	}
}

func TestParseRelativeMonthYearRoll(t *testing.T) {
	// January + month-1 → December previous year
	now := d("2024-01-10")
	r, err := Parse("month-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Start.Equal(d("2023-12-01")) || !r.End.Equal(d("2023-12-31")) {
		t.Fatalf("month-1 from Jan: %v %v", r.Start, r.End)
	}
	// Q1 - 1 → Q4 previous year
	r, err = Parse("quarter-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Start.Equal(d("2023-10-01")) || !r.End.Equal(d("2023-12-31")) {
		t.Fatalf("quarter-1 from Q1: %v %v", r.Start, r.End)
	}
}

func TestDisplayLabelKindAt(t *testing.T) {
	now := d("2024-07-15")

	t.Run("Kind", func(t *testing.T) {
		cases := []struct {
			filter string
			want   string
		}{
			{"", KindEmpty},
			{"  ", KindEmpty},
			{"2024", KindYear},
			{"2024-Q2", KindQuarter},
			{"2024-03", KindMonth},
			{"2024-W12", KindWeek},
			{"2024-07-15", KindDay},
			{"year", KindYear},
			{"year-1", KindYear},
			{"quarter+1", KindQuarter},
			{"month", KindMonth},
			{"week-2", KindWeek},
			{"day", KindDay},
			{"2020 - 2024", KindCustom},
			{"garbage", KindCustom},
		}
		for _, tc := range cases {
			if got := Kind(tc.filter, now); got != tc.want {
				t.Errorf("Kind(%q)=%q want %q", tc.filter, got, tc.want)
			}
		}
	})

	t.Run("At", func(t *testing.T) {
		anchor := d("2024-07-15")
		cases := []struct {
			kind string
			want string
		}{
			{KindYear, "2024"},
			{KindQuarter, "2024-Q3"},
			{KindMonth, "2024-07"},
			{KindWeek, "2024-W29"},
			{KindDay, "2024-07-15"},
			{"", "2024-07"},         // default month
			{KindCustom, "2024-07"}, // default month
		}
		for _, tc := range cases {
			if got := At(tc.kind, anchor); got != tc.want {
				t.Errorf("At(%q)=%q want %q", tc.kind, got, tc.want)
			}
		}
	})

	t.Run("DisplayLabel", func(t *testing.T) {
		cases := []struct {
			filter string
			want   string
		}{
			{"", "All time"},
			{"2024", "2024"},
			{"2024-Q2", "Q2 2024"},
			{"2024-03", "March 2024"},
			{"2024-W29", "Week 29, 2024"},
			{"2024-07-15", "Mon 15 Jul 2024"},
			{"month", "July 2024"},
			{"year-1", "2023"},
			// custom kind uses Range.Label, which prefers Raw when set
			{"2023 - 2024-03", "2023 - 2024-03"},
			{"not-valid", "not-valid"}, // parse error → raw
		}
		for _, tc := range cases {
			if got := DisplayLabel(tc.filter, now); got != tc.want {
				t.Errorf("DisplayLabel(%q)=%q want %q", tc.filter, got, tc.want)
			}
		}
	})
}

func TestDateOnlyAndAnchor(t *testing.T) {
	if !DateOnly(time.Time{}).IsZero() {
		t.Fatal("zero stays zero")
	}
	ts := time.Date(2024, 7, 15, 13, 45, 0, 0, time.FixedZone("X", -3*3600))
	got := DateOnly(ts)
	// Year/Month/Day taken from local wall of ts, then stored as UTC midnight
	want := time.Date(2024, 7, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("DateOnly=%v want %v", got, want)
	}

	now := d("2024-07-15")
	if a := AnchorDate("", now); !a.Equal(now) {
		t.Fatalf("empty anchor=%v", a)
	}
	if a := AnchorDate("2024-03", now); !a.Equal(d("2024-03-01")) {
		t.Fatalf("month anchor=%v", a)
	}
	if a := AnchorDate("bad", now); !a.Equal(now) {
		t.Fatalf("bad filter falls back to now: %v", a)
	}
}
