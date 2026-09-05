package period

import (
	"fmt"
	"strings"
	"time"
)

// Interval kinds used in the navbar (Fava-style).
const (
	KindEmpty   = ""
	KindYear    = "year"
	KindQuarter = "quarter"
	KindMonth   = "month"
	KindWeek    = "week"
	KindDay     = "day"
	KindCustom  = "custom"
)

// Kind returns the interval kind of a filter expression.
func Kind(filter string, now time.Time) string {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return KindEmpty
	}
	if strings.Contains(filter, " - ") {
		return KindCustom
	}
	if reRel.MatchString(filter) {
		m := reRel.FindStringSubmatch(filter)
		return m[1]
	}
	switch {
	case reDay.MatchString(filter):
		return KindDay
	case reWeek.MatchString(filter):
		return KindWeek
	case reMonth.MatchString(filter):
		return KindMonth
	case reQuarter.MatchString(filter):
		return KindQuarter
	case reYear.MatchString(filter):
		return KindYear
	default:
		return KindCustom
	}
}

// At returns the absolute filter expression of the given kind covering t.
func At(kind string, t time.Time) string {
	t = DateOnly(t.UTC())
	switch kind {
	case KindYear:
		return fmt.Sprintf("%04d", t.Year())
	case KindQuarter:
		q := (int(t.Month())-1)/3 + 1
		return fmt.Sprintf("%04d-Q%d", t.Year(), q)
	case KindMonth:
		return fmt.Sprintf("%04d-%02d", t.Year(), int(t.Month()))
	case KindWeek:
		y, w := t.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", y, w)
	case KindDay:
		return t.Format("2006-01-02")
	default:
		return fmt.Sprintf("%04d-%02d", t.Year(), int(t.Month()))
	}
}

// AnchorDate is a representative date inside the filter (start, or now if empty).
func AnchorDate(filter string, now time.Time) time.Time {
	now = DateOnly(now.UTC())
	if strings.TrimSpace(filter) == "" {
		return now
	}
	r, err := Parse(filter, now)
	if err != nil || r.Start.IsZero() {
		return now
	}
	return r.Start
}

// SetInterval re-expresses the filter in a new interval kind around the same anchor.
func SetInterval(filter string, now time.Time, kind string) string {
	if kind == KindEmpty || kind == "all" {
		return ""
	}
	return At(kind, AnchorDate(filter, now))
}

// Shift moves the filter by delta steps of its natural interval (±1 month, year, …).
// Empty filter starts from the current period of the given fallback kind (default month).
func Shift(filter string, now time.Time, delta int) (string, error) {
	now = DateOnly(now.UTC())
	filter = strings.TrimSpace(filter)
	kind := Kind(filter, now)
	if kind == KindEmpty {
		kind = KindMonth
		filter = At(kind, now)
	}
	if kind == KindCustom {
		// Shift custom ranges by whole months using start date.
		r, err := Parse(filter, now)
		if err != nil {
			return "", err
		}
		if r.Start.IsZero() {
			return "", fmt.Errorf("cannot shift open-ended range")
		}
		// keep span length in days
		span := 0
		if !r.End.IsZero() {
			span = int(r.End.Sub(r.Start).Hours()/24) + 1
		}
		start := r.Start.AddDate(0, delta, 0)
		if span > 0 {
			end := start.AddDate(0, 0, span-1)
			return start.Format("2006-01-02") + " - " + end.Format("2006-01-02"), nil
		}
		return start.Format("2006-01-02"), nil
	}

	// Resolve relative to absolute first
	r, err := Parse(filter, now)
	if err != nil {
		return "", err
	}
	if r.Start.IsZero() {
		return "", fmt.Errorf("cannot shift empty period")
	}
	anchor := r.Start
	switch kind {
	case KindYear:
		return At(KindYear, anchor.AddDate(delta, 0, 0)), nil
	case KindQuarter:
		return At(KindQuarter, anchor.AddDate(0, 3*delta, 0)), nil
	case KindMonth:
		return At(KindMonth, anchor.AddDate(0, delta, 0)), nil
	case KindWeek:
		return At(KindWeek, anchor.AddDate(0, 0, 7*delta)), nil
	case KindDay:
		return At(KindDay, anchor.AddDate(0, 0, delta)), nil
	default:
		return At(KindMonth, anchor.AddDate(0, delta, 0)), nil
	}
}

// DisplayLabel is a human label for the filter (e.g. "March 2024").
func DisplayLabel(filter string, now time.Time) string {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return "All time"
	}
	r, err := Parse(filter, now)
	if err != nil {
		return filter
	}
	kind := Kind(filter, now)
	if r.Start.IsZero() {
		return r.Label()
	}
	switch kind {
	case KindYear:
		return r.Start.Format("2006")
	case KindQuarter:
		q := (int(r.Start.Month())-1)/3 + 1
		return fmt.Sprintf("Q%d %d", q, r.Start.Year())
	case KindMonth:
		return r.Start.Format("January 2006")
	case KindWeek:
		y, w := r.Start.ISOWeek()
		return fmt.Sprintf("Week %d, %d", w, y)
	case KindDay:
		return r.Start.Format("Mon 2 Jan 2006")
	default:
		return r.Label()
	}
}
