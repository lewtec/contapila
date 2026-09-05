package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/contapila-go/pkg/project"
)

func TestPlanDesktopRewrite_TTY(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                string
		stdinTTY, stdoutTTY bool
	}{
		{"both TTY", true, true},
		{"stdin only", true, false},
		{"stdout only", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, ok := planDesktopRewrite(tc.stdinTTY, tc.stdoutTTY, nil)
			if ok {
				t.Fatal("expected no rewrite when any fd is a TTY")
			}
		})
	}
}

func TestPlanDesktopRewrite_NotTTY(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cue := filepath.Join(dir, project.ProjectMarker)
	if err := os.WriteFile(cue, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Nested start dir for walk-up is not required here; resolve only needs the path to exist.
	sub := filepath.Join(dir, "ledger")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	absCue, err := filepath.Abs(cue)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		args        []string
		wantOK      bool
		wantArgs    []string
		wantWorkDir string // empty = no override
	}{
		{
			name:     "zero args",
			args:     nil,
			wantOK:   true,
			wantArgs: []string{"desktop"},
		},
		{
			name:     "verbose only",
			args:     []string{"-v"},
			wantOK:   true,
			wantArgs: []string{"-v", "desktop"},
		},
		{
			name:     "directory flag",
			args:     []string{"-C", dir, "--verbose"},
			wantOK:   true,
			wantArgs: []string{"-C", dir, "--verbose", "desktop"},
		},
		{
			name:        "project dir positional",
			args:        []string{dir},
			wantOK:      true,
			wantArgs:    []string{"desktop"},
			wantWorkDir: absDir,
		},
		{
			name:        "cue file positional",
			args:        []string{cue},
			wantOK:      true,
			wantArgs:    []string{"desktop"},
			wantWorkDir: filepath.Dir(absCue),
		},
		{
			name:        "flags then project path",
			args:        []string{"-v", dir},
			wantOK:      true,
			wantArgs:    []string{"-v", "desktop"},
			wantWorkDir: absDir,
		},
		{
			name:   "real subcommand",
			args:   []string{"status"},
			wantOK: false,
		},
		{
			name:   "web subcommand",
			args:   []string{"web"},
			wantOK: false,
		},
		{
			name:   "two positionals",
			args:   []string{dir, "personal"},
			wantOK: false,
		},
		{
			name:   "missing path",
			args:   []string{filepath.Join(dir, "no-such")},
			wantOK: false,
		},
		{
			name:        "nested dir positional",
			args:        []string{sub},
			wantOK:      true,
			wantArgs:    []string{"desktop"},
			wantWorkDir: mustAbs(t, sub),
		},
		{
			name:   "unknown flag",
			args:   []string{"--addr", "127.0.0.1:1"},
			wantOK: false,
		},
		{
			name:   "incomplete -C",
			args:   []string{"-C"},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotArgs, gotDir, ok := planDesktopRewrite(false, false, tc.args)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v (args=%v dir=%q)", ok, tc.wantOK, gotArgs, gotDir)
			}
			if !ok {
				return
			}
			if !stringSlicesEqual(gotArgs, tc.wantArgs) {
				t.Errorf("args=%v want %v", gotArgs, tc.wantArgs)
			}
			if gotDir != tc.wantWorkDir {
				t.Errorf("workDir=%q want %q", gotDir, tc.wantWorkDir)
			}
		})
	}
}

func TestResolveProjectStartArg(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cue := filepath.Join(root, project.ProjectMarker)
	if err := os.WriteFile(cue, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}

	dir, ok := resolveProjectStartArg(root)
	if !ok || dir != absRoot {
		t.Fatalf("dir: got (%q, %v) want (%q, true)", dir, ok, absRoot)
	}
	dir, ok = resolveProjectStartArg(cue)
	if !ok || dir != absRoot {
		t.Fatalf("cue: got (%q, %v) want (%q, true)", dir, ok, absRoot)
	}
	if _, ok := resolveProjectStartArg(other); ok {
		t.Fatal("non-cue file should not resolve")
	}
	if _, ok := resolveProjectStartArg(filepath.Join(root, "missing")); ok {
		t.Fatal("missing path should not resolve")
	}
	if _, ok := resolveProjectStartArg(""); ok {
		t.Fatal("empty should not resolve")
	}
}

func TestProjectHasLedger(t *testing.T) {
	t.Parallel()
	p := &project.Project{
		Ledgers: []project.Ledger{
			{Name: "personal"},
			{Name: "acme"},
		},
	}
	if !projectHasLedger(p, "personal") {
		t.Fatal("expected personal")
	}
	if projectHasLedger(p, "missing") {
		t.Fatal("missing should be false")
	}
	if projectHasLedger(nil, "personal") {
		t.Fatal("nil project")
	}
}

func TestRootDeepLinkHandler(t *testing.T) {
	t.Parallel()
	var hit string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	h := rootDeepLinkHandler(next, "personal")

	// Root + token query (eletrocromo launch URL) → ledger check, query kept.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?token=abc", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("root status=%d want 302", rr.Code)
	}
	loc := rr.Header().Get("Location")
	want := "/l/personal/check?token=abc"
	if loc != want {
		t.Fatalf("Location=%q want %q", loc, want)
	}
	if hit != "" {
		t.Fatalf("next should not run on root redirect, hit=%q", hit)
	}

	// Non-root path passes through.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/l/personal/balances", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("pass-through status=%d", rr.Code)
	}
	if hit != "/l/personal/balances" {
		t.Fatalf("hit=%q", hit)
	}

	// Ledger names with special characters are path-escaped.
	h2 := rootDeepLinkHandler(next, "a/b")
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	h2.ServeHTTP(rr, req)
	if got := rr.Header().Get("Location"); got != "/l/a%2Fb/check" {
		t.Fatalf("escaped Location=%q", got)
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
