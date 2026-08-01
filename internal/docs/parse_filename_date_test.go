package docs

import (
	"strings"
	"testing"
	"time"
)

func TestParseFilenameDate(t *testing.T) {
	cases := []struct {
		name    string
		base    string
		want    time.Time // zero if expect error
		wantErr string    // substring; empty if expect success
	}{
		// Full yyyymmdd
		{
			name: "full date underscore",
			base: "20240315_statement.txt",
			want: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "full date dash rest",
			base: "20230810-INV-001.pdf",
			want: time.Date(2023, 8, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "full date no separator",
			base: "20240101note",
			want: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},

		// yyyymm → day defaults to 01
		{
			name: "year month only",
			base: "202403_statement.txt",
			want: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "year month january",
			base: "202401-report.pdf",
			want: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},

		// yyyy → month and day default to 01
		{
			name: "year only",
			base: "2024_annual.pdf",
			want: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "year only bare",
			base: "2024",
			want: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},

		// Invalid calendar
		{
			name:    "invalid month in yyyymmdd",
			base:    "20241301_x.txt",
			wantErr: "not a valid calendar date",
		},
		{
			name:    "invalid day",
			base:    "20240230_x.txt",
			wantErr: "not a valid calendar date",
		},
		{
			name:    "invalid month in yyyymm",
			base:    "202413_x.txt",
			wantErr: "not a valid calendar date",
		},

		// Wrong digit-run length (not yyyy / yyyymm / yyyymmdd before non-digit)
		{
			name:    "five digits",
			base:    "20240_x.txt",
			wantErr: "must start with yyyy",
		},
		{
			name:    "seven digits",
			base:    "2024031_x.txt",
			wantErr: "must start with yyyy",
		},
		{
			name:    "three digits",
			base:    "202_x.txt",
			wantErr: "must start with yyyy",
		},

		// No leading date
		{
			name:    "no digits",
			base:    "readme.txt",
			wantErr: "must start with yyyy",
		},
		{
			name:    "letters first",
			base:    "note-20240301.txt",
			wantErr: "must start with yyyy",
		},

		// Dashed civil date: only the leading digit run counts (year).
		{
			name: "dashed civil date takes year only",
			base: "2024-03-01_x.txt",
			want: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, msg := parseFilenameDate(tc.base)
			if tc.wantErr != "" {
				if msg == "" {
					t.Fatalf("base=%q: want error containing %q, got date %v", tc.base, tc.wantErr, got)
				}
				if !strings.Contains(msg, tc.wantErr) {
					t.Fatalf("base=%q: msg=%q want substring %q", tc.base, msg, tc.wantErr)
				}
				return
			}
			if msg != "" {
				t.Fatalf("base=%q: unexpected error %q", tc.base, msg)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("base=%q: got %v want %v", tc.base, got, tc.want)
			}
		})
	}
}
