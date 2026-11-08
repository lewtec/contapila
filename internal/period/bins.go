package period

import (
	"strings"
	"time"
)

// BinKind is the calendar grain for P&L bar charts (Fava-style from filter).
type BinKind int

const (
	BinDay BinKind = iota
	BinWeek
	BinMonth
	BinYear
)

func (k BinKind) String() string {
	switch k {
	case BinDay:
		return "day"
	case BinWeek:
		return "week"
	case BinYear:
		return "year"
	default:
		return "month"
	}
}

// ChartBin chooses bar bin size from the time filter string and resolved range.
// Rules (Fava-ish + multi-year → year):
//
//	year token → month
//	quarter → month
//	month / week / day → day
//	range spanning ≥ 2 calendar years → year
//	other multi-month range → month
//	empty / all time with multi-year span → year; else month
func ChartBin(raw string, r Range) BinKind {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return binFromSpan(r)
	}
	// Explicit range "A - B"
	if strings.Contains(raw, " - ") {
		return binFromSpan(r)
	}
	// Relative / absolute single tokens
	if reDay.MatchString(raw) || strings.HasPrefix(raw, "day") {
		return BinDay
	}
	if reWeek.MatchString(raw) || strings.HasPrefix(raw, "week") {
		return BinDay
	}
	if reMonth.MatchString(raw) || strings.HasPrefix(raw, "month") {
		return BinDay
	}
	if reQuarter.MatchString(raw) || strings.HasPrefix(raw, "quarter") {
		return BinMonth
	}
	if reYear.MatchString(raw) || strings.HasPrefix(raw, "year") {
		return BinMonth
	}
	return binFromSpan(r)
}

func binFromSpan(r Range) BinKind {
	if r.Start.IsZero() || r.End.IsZero() {
		// open-ended: prefer month unless clearly multi-year when both ends known later
		return BinMonth
	}
	// ≥ 2 calendar years between start and end → year bins
	if r.End.Year()-r.Start.Year() >= 2 {
		return BinYear
	}
	// single day
	if sameDay(r.Start, r.End) {
		return BinDay
	}
	// less than ~2 months → day
	if r.End.Before(r.Start.AddDate(0, 2, 0)) {
		return BinDay
	}
	return BinMonth
}

// IterateBins yields [start, endInclusive] bins covering r for kind.
// If r is open on a side, caller should clamp first.
func IterateBins(r Range, kind BinKind) []Range {
	if r.Start.IsZero() || r.End.IsZero() {
		return nil
	}
	start := DateOnly(r.Start)
	end := DateOnly(r.End)
	if start.After(end) {
		return nil
	}
	var out []Range
	cur := startOfBin(start, kind)
	for !cur.After(end) {
		next := nextBin(cur, kind)
		binEnd := next.AddDate(0, 0, -1)
		if binEnd.After(end) {
			binEnd = end
		}
		binStart := cur
		if binStart.Before(start) {
			binStart = start
		}
		if !binStart.After(binEnd) {
			out = append(out, Range{Start: binStart, End: binEnd})
		}
		cur = next
		if len(out) > 5000 {
			break
		}
	}
	return out
}

func startOfBin(t time.Time, kind BinKind) time.Time {
	t = DateOnly(t)
	switch kind {
	case BinDay:
		return t
	case BinWeek:
		// Monday
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		return t.AddDate(0, 0, 1-wd)
	case BinYear:
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	default: // month
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
}

func nextBin(start time.Time, kind BinKind) time.Time {
	switch kind {
	case BinDay:
		return start.AddDate(0, 0, 1)
	case BinWeek:
		return start.AddDate(0, 0, 7)
	case BinYear:
		return start.AddDate(1, 0, 0)
	default:
		return start.AddDate(0, 1, 0)
	}
}
