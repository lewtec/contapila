package diag

import "testing"

func TestListHasErrorsAndWarnings(t *testing.T) {
	tests := []struct {
		name         string
		list         List
		wantErrors   bool
		wantWarnings bool
	}{
		{
			name:         "empty",
			list:         nil,
			wantErrors:   false,
			wantWarnings: false,
		},
		{
			name: "only errors",
			list: List{
				{Severity: Error, Message: "bad"},
			},
			wantErrors:   true,
			wantWarnings: false,
		},
		{
			name: "only warnings",
			list: List{
				{Severity: Warn, Message: "soft"},
			},
			wantErrors:   false,
			wantWarnings: true,
		},
		{
			name: "mixed",
			list: List{
				{Severity: Warn, Message: "soft"},
				{Severity: Error, Message: "bad"},
			},
			wantErrors:   true,
			wantWarnings: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.list.HasErrors(); got != tt.wantErrors {
				t.Errorf("HasErrors() = %v; want %v", got, tt.wantErrors)
			}
			if got := tt.list.HasWarnings(); got != tt.wantWarnings {
				t.Errorf("HasWarnings() = %v; want %v", got, tt.wantWarnings)
			}
		})
	}
}

func TestDiagnosticLocation(t *testing.T) {
	tests := []struct {
		name string
		d    Diagnostic
		want string
	}{
		{
			name: "file and line",
			d:    Diagnostic{File: "main.beancount", Line: 12},
			want: "main.beancount:12",
		},
		{
			name: "file only",
			d:    Diagnostic{File: "main.beancount", Line: 0},
			want: "main.beancount",
		},
		{
			name: "empty file",
			d:    Diagnostic{File: "", Line: 5},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.d.Location(); got != tt.want {
				t.Errorf("Location() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestDiagnosticString(t *testing.T) {
	tests := []struct {
		name string
		d    Diagnostic
		want string
	}{
		{
			name: "file and line",
			d:    Diagnostic{Severity: Error, Message: "bad", File: "main.beancount", Line: 12},
			want: "main.beancount:12: error: bad",
		},
		{
			name: "file only",
			d:    Diagnostic{Severity: Warn, Message: "soft", File: "main.beancount", Line: 0},
			want: "main.beancount: warn: soft",
		},
		{
			name: "empty file",
			d:    Diagnostic{Severity: Error, Message: "bad", File: "", Line: 0},
			want: "error: bad",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.d.String(); got != tt.want {
				t.Errorf("String() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestSeverityString(t *testing.T) {
	tests := []struct {
		name string
		s    Severity
		want string
	}{
		{name: "error", s: Error, want: "error"},
		{name: "warn", s: Warn, want: "warn"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.s.String(); got != tt.want {
				t.Errorf("Severity.String() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestListFormat(t *testing.T) {
	tests := []struct {
		name string
		list List
		want string
	}{
		{
			name: "empty",
			list: nil,
			want: "",
		},
		{
			name: "single warning",
			list: List{
				{Severity: Warn, Message: "soft", File: "a.beancount", Line: 1},
			},
			want: "a.beancount:1: warn: soft",
		},
		{
			name: "mixed preserves order",
			list: List{
				{Severity: Warn, Message: "soft", File: "a.beancount", Line: 1},
				{Severity: Error, Message: "first", File: "b.beancount", Line: 2},
				{Severity: Error, Message: "second", File: "c.beancount", Line: 3},
			},
			want: "a.beancount:1: warn: soft\nb.beancount:2: error: first\nc.beancount:3: error: second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.list.Format(); got != tt.want {
				t.Errorf("Format() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestListFormatErrors(t *testing.T) {
	tests := []struct {
		name string
		list List
		want string
	}{
		{
			name: "empty",
			list: nil,
			want: "",
		},
		{
			name: "warnings only",
			list: List{
				{Severity: Warn, Message: "soft", File: "a.beancount", Line: 1},
			},
			want: "",
		},
		{
			name: "single error",
			list: List{
				{Severity: Error, Message: "bad", File: "main.beancount", Line: 12},
			},
			want: "main.beancount:12: error: bad",
		},
		{
			name: "mixed skips warnings",
			list: List{
				{Severity: Warn, Message: "soft", File: "a.beancount", Line: 1},
				{Severity: Error, Message: "first", File: "b.beancount", Line: 2},
				{Severity: Error, Message: "second", File: "c.beancount", Line: 3},
			},
			want: "b.beancount:2: error: first; c.beancount:3: error: second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.list.FormatErrors(); got != tt.want {
				t.Errorf("FormatErrors() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestListWarnErrorMerge(t *testing.T) {
	var l List
	l.Warn("a.beancount", 1, "soft")
	l.Error("b.beancount", 2, "bad")

	if len(l) != 2 {
		t.Fatalf("after Warn/Error len = %d; want 2", len(l))
	}
	if l[0].Severity != Warn || l[0].File != "a.beancount" || l[0].Line != 1 || l[0].Message != "soft" {
		t.Errorf("Warn append = %+v; want warn a.beancount:1 soft", l[0])
	}
	if l[1].Severity != Error || l[1].File != "b.beancount" || l[1].Line != 2 || l[1].Message != "bad" {
		t.Errorf("Error append = %+v; want error b.beancount:2 bad", l[1])
	}

	other := List{
		{Severity: Warn, Message: "more", File: "c.beancount", Line: 3},
	}
	l.Merge(other)
	if len(l) != 3 {
		t.Fatalf("after Merge len = %d; want 3", len(l))
	}
	if l[2].Severity != Warn || l[2].File != "c.beancount" || l[2].Line != 3 || l[2].Message != "more" {
		t.Errorf("Merge append = %+v; want warn c.beancount:3 more", l[2])
	}
	if !l.HasErrors() || !l.HasWarnings() {
		t.Errorf("after helpers HasErrors=%v HasWarnings=%v; want true,true", l.HasErrors(), l.HasWarnings())
	}
}
