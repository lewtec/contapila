package lsp

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func isAccountRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == ':' || r == '-' || r == '_'
}

func isCommodityRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.'
}

// tokenAt returns the identifier under byte offset.
func tokenAt(text string, byteOff int) (tok string, start, end int) {
	if byteOff < 0 {
		byteOff = 0
	}
	if byteOff > len(text) {
		byteOff = len(text)
	}
	start, end = byteOff, byteOff
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:start])
		if r == utf8.RuneError && size == 1 {
			break
		}
		if !isAccountRune(r) && !isCommodityRune(r) {
			break
		}
		start -= size
	}
	for end < len(text) {
		r, size := utf8.DecodeRuneInString(text[end:])
		if r == utf8.RuneError && size == 1 {
			break
		}
		if !isAccountRune(r) && !isCommodityRune(r) {
			break
		}
		end += size
	}
	return text[start:end], start, end
}

// completionKind classifies cursor context: "date", "account", "commodity", or "".
func completionKind(linePrefix string) string {
	trimmed := strings.TrimSpace(linePrefix)
	if trimmed == "" {
		// blank posting line indentation
		if len(linePrefix) > 0 && (linePrefix[0] == ' ' || linePrefix[0] == '\t') {
			return "account"
		}
		// top-level empty / only spaces before first token → date directive
		return "date"
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return "date"
	}

	// Typing a date (or date prefix) as the first field on a non-indented line.
	if len(linePrefix) == 0 || (linePrefix[0] != ' ' && linePrefix[0] != '\t') {
		if len(fields) == 1 && isDatePrefix(fields[0]) && !looksDate(fields[0]) {
			return "date"
		}
		// complete date alone (user may still want to replace / continue) — not date complete
		if len(fields) == 1 && isDatePrefix(fields[0]) && looksDate(fields[0]) {
			// after a full date, next is keyword — no date items
			return ""
		}
	}

	// "open Assets:…" without date, or "2020-01-01 open …"
	kwIdx := 0
	if looksDate(fields[0]) && len(fields) >= 2 {
		kwIdx = 1
	}
	if kwIdx < len(fields) {
		switch fields[kwIdx] {
		case "open", "close", "balance", "pad", "document", "note":
			return "account"
		case "commodity", "price":
			return "commodity"
		case "*", "!", "txn":
			return ""
		}
	}

	// Posting lines are indented.
	if len(linePrefix) > 0 && (linePrefix[0] == ' ' || linePrefix[0] == '\t') {
		parts := strings.Fields(linePrefix)
		if len(parts) == 0 {
			return "account"
		}
		if len(parts) == 1 {
			return "account"
		}
		// After account + number → commodity
		if len(parts) >= 2 && looksNumber(parts[1]) {
			return "commodity"
		}
		last := parts[len(parts)-1]
		if looksNumber(last) {
			return "commodity"
		}
		if !strings.Contains(parts[0], ":") && len(parts) > 1 {
			return "commodity"
		}
		return "account"
	}
	return ""
}

func looksDate(s string) bool {
	// YYYY-MM-DD
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, c := range s {
		if i == 4 || i == 7 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isDatePrefix reports a partial or full ISO calendar date token (digits and '-').
// Examples: "2", "2024", "2024-", "2024-01", "2024-01-1", "2024-01-15".
func isDatePrefix(s string) bool {
	if s == "" || len(s) > 10 {
		return false
	}
	digits, dashes := 0, 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c == '-':
			dashes++
			if dashes > 2 {
				return false
			}
			// dashes only after year or month groups
			if i != 4 && i != 7 {
				return false
			}
		default:
			return false
		}
	}
	if digits == 0 {
		return false
	}
	// reject pure numbers that look like amounts with decimal
	if strings.Contains(s, ".") {
		return false
	}
	return true
}

func looksNumber(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '-' || s[0] == '+' {
		i++
	}
	if i >= len(s) {
		return false
	}
	dots := 0
	for ; i < len(s); i++ {
		c := s[i]
		if c == '.' {
			dots++
			if dots > 1 {
				return false
			}
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func linePrefixAt(text string, byteOff int) string {
	if byteOff > len(text) {
		byteOff = len(text)
	}
	start := byteOff
	for start > 0 && text[start-1] != '\n' {
		start--
	}
	return text[start:byteOff]
}
