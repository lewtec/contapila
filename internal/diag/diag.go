package diag

import (
	"fmt"
	"strings"
)

type Severity int

const (
	Warn Severity = iota
	Error
)

func (s Severity) String() string {
	if s == Error {
		return "error"
	}
	return "warn"
}

type Diagnostic struct {
	Severity Severity
	Message  string
	File     string
	Line     int
}

func (d Diagnostic) IsError() bool { return d.Severity == Error }
func (d Diagnostic) IsWarn() bool  { return d.Severity == Warn }

// Location is path:line when line is known, else path (or empty).
func (d Diagnostic) Location() string {
	if d.File == "" {
		return ""
	}
	if d.Line > 0 {
		return fmt.Sprintf("%s:%d", d.File, d.Line)
	}
	return d.File
}

func (d Diagnostic) String() string {
	loc := d.Location()
	if loc == "" {
		return fmt.Sprintf("%s: %s", d.Severity, d.Message)
	}
	return fmt.Sprintf("%s: %s: %s", loc, d.Severity, d.Message)
}

type List []Diagnostic

func (l List) HasErrors() bool {
	for _, d := range l {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

func (l List) HasWarnings() bool {
	for _, d := range l {
		if d.Severity == Warn {
			return true
		}
	}
	return false
}

// Format joins all diagnostics via Diagnostic.String(), one per line.
// Empty lists return "". Prefer this for multi-line CLI/stderr output.
func (l List) Format() string {
	if len(l) == 0 {
		return ""
	}
	parts := make([]string, len(l))
	for i, d := range l {
		parts[i] = d.String()
	}
	return strings.Join(parts, "\n")
}

// FormatErrors joins error-severity diagnostics via Diagnostic.String(),
// separated by "; " for compact single-line error text (e.g. fmt.Errorf).
// Warnings are omitted.
func (l List) FormatErrors() string {
	var parts []string
	for _, d := range l {
		if d.IsError() {
			parts = append(parts, d.String())
		}
	}
	return strings.Join(parts, "; ")
}

// Warn appends a warning. line is 1-based; use 0 if unknown.
func (l *List) Warn(file string, line int, msg string) {
	*l = append(*l, Diagnostic{Severity: Warn, Message: msg, File: file, Line: line})
}

// Error appends an error. line is 1-based; use 0 if unknown.
func (l *List) Error(file string, line int, msg string) {
	*l = append(*l, Diagnostic{Severity: Error, Message: msg, File: file, Line: line})
}

func (l *List) Merge(other List) {
	*l = append(*l, other...)
}
