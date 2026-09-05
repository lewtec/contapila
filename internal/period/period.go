// Package period parses Fava-like time filters into inclusive date ranges.
//
// Examples (see Fava help/filters):
//
//	2024
//	2024-Q1
//	2024-03
//	2024-W12
//	2024-03-15
//	2020 - 2024-06
//	year, year-1, month, month-1, quarter, week, day
package period

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Range is an inclusive calendar range. Zero Start/End means open on that side.
type Range struct {
	Start time.Time // inclusive; zero = unbounded past
	End   time.Time // inclusive; zero = unbounded future
	Raw   string
}

func (r Range) Empty() bool { return r.Start.IsZero() && r.End.IsZero() }

func (r Range) Label() string {
	if r.Raw != "" {
		return r.Raw
	}
	if r.Empty() {
		return "all time"
	}
	if !r.Start.IsZero() && !r.End.IsZero() {
		if sameDay(r.Start, r.End) {
			return r.Start.Format("2006-01-02")
		}
		return r.Start.Format("2006-01-02") + " … " + r.End.Format("2006-01-02")
	}
	if !r.Start.IsZero() {
		return "from " + r.Start.Format("2006-01-02")
	}
	return "until " + r.End.Format("2006-01-02")
}

// Parse interprets a Fava-style time filter relative to now (UTC calendar date).
func Parse(s string, now time.Time) (Range, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Range{}, nil
	}
	now = DateOnly(now.UTC())

	// Range: "A - B" (spaces around hyphen, as in Fava).
	if i := strings.Index(s, " - "); i >= 0 {
		left := strings.TrimSpace(s[:i])
		right := strings.TrimSpace(s[i+3:])
		a, err := parseOne(left, now)
		if err != nil {
			return Range{}, fmt.Errorf("time range start %q: %w", left, err)
		}
		b, err := parseOne(right, now)
		if err != nil {
			return Range{}, fmt.Errorf("time range end %q: %w", right, err)
		}
		start, _ := a.bounds()
		_, end := b.bounds()
		if !start.IsZero() && !end.IsZero() && start.After(end) {
			return Range{}, fmt.Errorf("time range start after end: %s - %s", left, right)
		}
		return Range{Start: start, End: end, Raw: s}, nil
	}

	one, err := parseOne(s, now)
	if err != nil {
		return Range{}, err
	}
	start, end := one.bounds()
	return Range{Start: start, End: end, Raw: s}, nil
}

type interval struct {
	start        time.Time // inclusive
	endExclusive time.Time // exclusive
}

func (iv interval) bounds() (start, endIncl time.Time) {
	start = iv.start
	if !iv.endExclusive.IsZero() {
		endIncl = iv.endExclusive.AddDate(0, 0, -1)
	}
	return start, endIncl
}

var (
	reYear    = regexp.MustCompile(`^(\d{4})$`)
	reQuarter = regexp.MustCompile(`^(\d{4})-[Qq]([1-4])$`)
	reMonth   = regexp.MustCompile(`^(\d{4})-(\d{2})$`)
	reWeek    = regexp.MustCompile(`^(\d{4})-[Ww](\d{1,2})$`)
	reDay     = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	reRel     = regexp.MustCompile(`^(year|quarter|month|week|day)([+-]\d+)?$`)
)

func parseOne(s string, now time.Time) (interval, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return interval{}, fmt.Errorf("empty interval")
	}

	if m := reRel.FindStringSubmatch(s); m != nil {
		unit := m[1]
		off := 0
		if m[2] != "" {
			var err error
			off, err = strconv.Atoi(m[2])
			if err != nil {
				return interval{}, err
			}
		}
		return relative(unit, off, now)
	}

	if m := reDay.FindStringSubmatch(s); m != nil {
		t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
		if err != nil {
			return interval{}, err
		}
		t = DateOnly(t)
		return interval{start: t, endExclusive: t.AddDate(0, 0, 1)}, nil
	}
	if m := reWeek.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		w, _ := strconv.Atoi(m[2])
		if w < 1 || w > 53 {
			return interval{}, fmt.Errorf("invalid ISO week %d", w)
		}
		start := isoWeekStart(y, w)
		return interval{start: start, endExclusive: start.AddDate(0, 0, 7)}, nil
	}
	if m := reMonth.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		if mo < 1 || mo > 12 {
			return interval{}, fmt.Errorf("invalid month %d", mo)
		}
		start := time.Date(y, time.Month(mo), 1, 0, 0, 0, 0, time.UTC)
		return interval{start: start, endExclusive: start.AddDate(0, 1, 0)}, nil
	}
	if m := reQuarter.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		q, _ := strconv.Atoi(m[2])
		startMonth := time.Month(1 + (q-1)*3)
		start := time.Date(y, startMonth, 1, 0, 0, 0, 0, time.UTC)
		return interval{start: start, endExclusive: start.AddDate(0, 3, 0)}, nil
	}
	if m := reYear.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		start := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
		return interval{start: start, endExclusive: start.AddDate(1, 0, 0)}, nil
	}

	return interval{}, fmt.Errorf("unrecognized time filter %q (try 2024, 2024-03, 2024-Q1, month, month-1, year)", s)
}

func relative(unit string, offset int, now time.Time) (interval, error) {
	switch unit {
	case "day":
		d := now.AddDate(0, 0, offset)
		return interval{start: d, endExclusive: d.AddDate(0, 0, 1)}, nil
	case "week":
		// ISO week containing now, shifted by offset weeks
		y, w := now.ISOWeek()
		start := isoWeekStart(y, w).AddDate(0, 0, 7*offset)
		return interval{start: start, endExclusive: start.AddDate(0, 0, 7)}, nil
	case "month":
		first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, offset, 0)
		return interval{start: first, endExclusive: first.AddDate(0, 1, 0)}, nil
	case "quarter":
		q := (int(now.Month())-1)/3 + 1
		// express as months from year start, then apply offset in quarters
		startMonth := time.Month(1 + (q-1)*3)
		first := time.Date(now.Year(), startMonth, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 3*offset, 0)
		return interval{start: first, endExclusive: first.AddDate(0, 3, 0)}, nil
	case "year":
		first := time.Date(now.Year()+offset, 1, 1, 0, 0, 0, 0, time.UTC)
		return interval{start: first, endExclusive: first.AddDate(1, 0, 0)}, nil
	default:
		return interval{}, fmt.Errorf("unknown relative unit %q", unit)
	}
}

// isoWeekStart returns Monday 00:00 UTC of ISO week w of year y.
func isoWeekStart(year, week int) time.Time {
	// Jan 4 is always in week 1
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	// Monday of week 1
	wd := int(jan4.Weekday())
	if wd == 0 {
		wd = 7
	}
	week1Mon := jan4.AddDate(0, 0, 1-wd)
	return week1Mon.AddDate(0, 0, (week-1)*7)
}

// DateOnly returns the UTC calendar day of t (00:00:00). Zero times stay zero.
func DateOnly(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// ParseDate parses a YYYY-MM-DD calendar date in UTC.
// Empty s yields the zero time and a nil error (open/unbounded as-of style).
func ParseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.ParseInLocation("2006-01-02", s, time.UTC)
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}
