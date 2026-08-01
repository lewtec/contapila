package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
)

func TestScanByAccount(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "personal", "docs", "by-account", "Assets", "BR", "Cash")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20240301_statement.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "acme", "docs", "by-account", "Assets", "Cash")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "20240101_x.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, diags, err := ScanByAccount(root, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if len(got) != 1 {
		t.Fatalf("got %d docs: %+v", len(got), got)
	}
	d := got[0]
	if d.Account != "Assets:BR:Cash" {
		t.Fatalf("account=%q", d.Account)
	}
	want := "personal/docs/by-account/Assets/BR/Cash/20240301_statement.txt"
	if d.Path != want {
		t.Fatalf("path=%q want %q", d.Path, want)
	}
	if !d.Synthetic {
		t.Fatal("expected synthetic")
	}
	if !d.Date.Equal(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date=%v", d.Date)
	}
}

func TestScanByAccount_badFilenames(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "personal", "docs", "by-account", "Assets", "Cash")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Valid companion kept.
	if err := os.WriteFile(filepath.Join(dir, "20240301_ok.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		want string // substring of diag message
	}{
		{"20241301_statement.txt", "20241301"},           // 8 digits, invalid calendar
		{"202403_statement.txt", "must start with yyyymmdd"}, // yyyymm only
		{"2024-03-01_x.txt", "must start with yyyymmdd"},
		{"readme.txt", "must start with yyyymmdd"},
	}
	for _, tc := range cases {
		if err := os.WriteFile(filepath.Join(dir, tc.name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, diags, err := ScanByAccount(root, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "personal/docs/by-account/Assets/Cash/20240301_ok.txt" {
		t.Fatalf("got %+v", got)
	}
	if len(diags) != len(cases) {
		t.Fatalf("diags count=%d want %d: %v", len(diags), len(cases), diags)
	}
	for _, tc := range cases {
		found := false
		wantFile := "personal/docs/by-account/Assets/Cash/" + tc.name
		for _, d := range diags {
			if d.File == wantFile {
				found = true
				if !strings.Contains(d.Message, tc.want) {
					t.Errorf("%s: message=%q want substring %q", tc.name, d.Message, tc.want)
				}
				if !d.IsError() {
					t.Errorf("%s: want error severity", tc.name)
				}
			}
		}
		if !found {
			t.Errorf("missing diag for %s in %v", tc.name, diags)
		}
	}
}

func TestMergePrefersExplicit(t *testing.T) {
	path := "personal/docs/by-account/Assets/Cash/20240101_x.txt"
	syn := []ast.Document{{
		Meta:    ast.Meta{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		Account: "Assets:Cash", Path: path, Synthetic: true,
	}}
	exp := []ast.Document{{
		Meta:    ast.Meta{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), File: "main.beancount"},
		Account: "Assets:Cash", Path: path, Synthetic: false,
	}}
	out := Merge(exp, syn)
	if len(out) != 1 || out[0].Synthetic || out[0].File != "main.beancount" {
		t.Fatalf("%+v", out)
	}
}

func TestIsLedgerDocPath(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"personal/docs/by-account/x", true},
		{"personal/docs/README.md", true},
		{"/personal/docs/x", true},
		{"personal/main.beancount", false},
		{"personal/docs", false},
		{"docs/foo/bar", false},
		{"", false},
		{".", false},
		{"../docs/x", false},
		{"personal/docs/../secret", false},
		{"personal/docs/foo/../../etc/passwd", false},
		{"personal/docs//x", false},
		{"//personal/docs/x", true},
		{"personal/./docs/x", true}, // "." collapsed by Clean
		{"acme/docs/by-account/a/b.txt", true},
	}
	for _, tc := range cases {
		if got := IsLedgerDocPath(tc.in); got != tc.want {
			t.Errorf("IsLedgerDocPath(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}
