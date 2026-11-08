package lsp

import (
	"regexp"
	"sort"
	"time"
)

var dateTokenRE = regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\b`)

// dateSuggestion is one completion row.
type dateSuggestion struct {
	Date   string // YYYY-MM-DD
	Detail string // "today", "yesterday", "in file", …
	Sort   string // sortText (lower = higher)
}

// suggestDates builds date completions matching prefix (may be empty or partial).
// now is injectable for tests; pass time.Time{} to use time.Now().
func suggestDates(prefix, docText string, now time.Time) []dateSuggestion {
	if now.IsZero() {
		now = time.Now()
	}
	now = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	type meta struct {
		detail string
		sort   string
	}
	cand := map[string]meta{}

	add := func(d time.Time, detail, sortKey string) {
		s := d.Format("2006-01-02")
		if prefix != "" && !hasDatePrefix(s, prefix) {
			return
		}
		if old, ok := cand[s]; ok {
			// keep better (lexicographically smaller) sort key
			if sortKey >= old.sort {
				return
			}
		}
		cand[s] = meta{detail: detail, sort: sortKey}
	}

	// Preferred top three (sort keys 0–2), then in-file dates.
	add(now, "today", "0")
	add(now.AddDate(0, 0, -1), "yesterday", "1")
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	add(monthStart, "start of month", "2")

	// dates already in the buffer (recent journal context)
	for _, m := range dateTokenRE.FindAllString(docText, -1) {
		if t, err := time.ParseInLocation("2006-01-02", m, now.Location()); err == nil {
			add(t, "in file", "5"+m) // stable among file dates
		}
	}

	out := make([]dateSuggestion, 0, len(cand))
	for d, m := range cand {
		out = append(out, dateSuggestion{Date: d, Detail: m.detail, Sort: m.sort})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sort != out[j].Sort {
			return out[i].Sort < out[j].Sort
		}
		return out[i].Date > out[j].Date // newer first when same tier
	})
	return out
}

func hasDatePrefix(full, prefix string) bool {
	if prefix == "" {
		return true
	}
	// allow prefix without requiring trailing chars: "2024-0" matches "2024-01-15"
	if len(prefix) > len(full) {
		return false
	}
	return full[:len(prefix)] == prefix
}
