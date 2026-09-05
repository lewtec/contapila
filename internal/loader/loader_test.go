package loader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
	"github.com/lucasew/contapila-go/internal/filesys"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func openAccounts(dirs []ast.Directive) []string {
	var accounts []string
	for _, d := range dirs {
		if o, ok := d.(ast.Open); ok {
			accounts = append(accounts, o.Account)
		}
	}
	return accounts
}

func countOpens(dirs []ast.Directive) int {
	n := 0
	for _, d := range dirs {
		if _, ok := d.(ast.Open); ok {
			n++
		}
	}
	return n
}

func hasDiagMsg(diags diag.List, severity diag.Severity, substr string) bool {
	for _, d := range diags {
		if d.Severity == severity && strings.Contains(d.Message, substr) {
			return true
		}
	}
	return false
}

func TestLoadFileSimpleNoIncludes(t *testing.T) {
	dir := t.TempDir()
	main := writeFile(t, dir, "main.beancount", `
2020-01-01 open Assets:Cash
2020-01-01 open Expenses:Food
`)

	dirs, diags, err := LoadFile(main)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	got := openAccounts(dirs)
	if len(got) != 2 || got[0] != "Assets:Cash" || got[1] != "Expenses:Food" {
		t.Fatalf("opens=%v want [Assets:Cash Expenses:Food]", got)
	}
	// Include directives are expanded away; none should remain in the stream.
	for _, d := range dirs {
		if _, ok := d.(ast.Include); ok {
			t.Fatalf("unexpected Include in stream: %+v", d)
		}
	}
}

func TestLoadFileMissingInclude(t *testing.T) {
	dir := t.TempDir()
	main := writeFile(t, dir, "main.beancount", `
include "missing.beancount"
2020-01-01 open Assets:Cash
`)

	dirs, diags, err := LoadFile(main)
	if err == nil {
		t.Fatal("expected error for missing include")
	}
	if !strings.Contains(err.Error(), "include missing") {
		t.Fatalf("err=%v want include missing", err)
	}
	if !hasDiagMsg(diags, diag.Error, "include missing") {
		t.Fatalf("expected error diagnostic for missing include, diags=%v", diags)
	}
	// Expansion aborts on missing include; nothing from after the include is loaded.
	if countOpens(dirs) != 0 {
		t.Fatalf("opens=%d want 0 after missing include abort", countOpens(dirs))
	}
}

func TestLoadFileIncludeCycle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.beancount", `
include "b.beancount"
2020-01-01 open Assets:A
`)
	writeFile(t, dir, "b.beancount", `
include "a.beancount"
2020-01-01 open Assets:B
`)
	main := filepath.Join(dir, "a.beancount")

	_, diags, err := LoadFile(main)
	if err == nil {
		t.Fatal("expected error for include cycle")
	}
	if !strings.Contains(err.Error(), "include cycle") {
		t.Fatalf("err=%v want include cycle", err)
	}
	if !hasDiagMsg(diags, diag.Error, "include cycle") {
		t.Fatalf("expected cycle diagnostic, diags=%v", diags)
	}
}

func TestLoadFileDedupeSameFileTwice(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "shared.beancount", `
2020-01-01 open Assets:Shared
`)
	main := writeFile(t, dir, "main.beancount", `
include "shared.beancount"
include "shared.beancount"
2020-01-01 open Assets:Main
`)

	dirs, diags, err := LoadFile(main)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	got := openAccounts(dirs)
	// shared once, then main — no duplicate Assets:Shared
	want := []string{"Assets:Shared", "Assets:Main"}
	if len(got) != len(want) {
		t.Fatalf("opens=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("opens=%v want %v", got, want)
		}
	}
}

func TestLoadFileDedupeDiamond(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "leaf.beancount", `
2020-01-01 open Assets:Leaf
`)
	writeFile(t, dir, "left.beancount", `
include "leaf.beancount"
2020-01-01 open Assets:Left
`)
	writeFile(t, dir, "right.beancount", `
include "leaf.beancount"
2020-01-01 open Assets:Right
`)
	main := writeFile(t, dir, "main.beancount", `
include "left.beancount"
include "right.beancount"
2020-01-01 open Assets:Main
`)

	dirs, diags, err := LoadFile(main)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	got := openAccounts(dirs)
	// leaf loaded via left only; right's re-include is deduped
	want := []string{"Assets:Leaf", "Assets:Left", "Assets:Right", "Assets:Main"}
	if len(got) != len(want) {
		t.Fatalf("opens=%v want %v (leaf should appear once)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("opens=%v want %v", got, want)
		}
	}
	// Explicit: leaf not duplicated
	nLeaf := 0
	for _, a := range got {
		if a == "Assets:Leaf" {
			nLeaf++
		}
	}
	if nLeaf != 1 {
		t.Fatalf("Assets:Leaf count=%d want 1", nLeaf)
	}
}

func TestLoadFileGlobZeroMatchesWarns(t *testing.T) {
	dir := t.TempDir()
	main := writeFile(t, dir, "main.beancount", `
include "no-such-*.beancount"
2020-01-01 open Assets:Cash
`)

	dirs, diags, err := LoadFile(main)
	if err != nil {
		t.Fatalf("LoadFile: %v (zero-match glob should not hard-fail)", err)
	}
	if diags.HasErrors() {
		t.Fatalf("zero-match glob should warn, not error: %v", diags)
	}
	if !hasDiagMsg(diags, diag.Warn, "include glob matched zero files") {
		t.Fatalf("expected zero-match warn, diags=%v", diags)
	}
	got := openAccounts(dirs)
	if len(got) != 1 || got[0] != "Assets:Cash" {
		t.Fatalf("opens=%v want [Assets:Cash]", got)
	}
}

func TestLoadFileGlobLoadsSorted(t *testing.T) {
	dir := t.TempDir()
	// Subdir so the entry file is not itself matched by the glob (matching the
	// still-loading root would hit stack-cycle before seen-dedupe).
	// Names chosen so lexical order is b then c.
	writeFile(t, dir, "parts/c.beancount", `
2020-01-01 open Assets:C
`)
	writeFile(t, dir, "parts/b.beancount", `
2020-01-01 open Assets:B
`)
	// Unrelated file must not match the glob.
	writeFile(t, dir, "parts/other.txt", "not beancount\n")
	main := writeFile(t, dir, "main.beancount", `
include "parts/*.beancount"
2020-01-01 open Assets:Main
`)

	dirs, diags, err := LoadFile(main)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	got := openAccounts(dirs)
	// Glob expands sorted: parts/b.beancount then parts/c.beancount.
	want := []string{"Assets:B", "Assets:C", "Assets:Main"}
	if len(got) != len(want) {
		t.Fatalf("opens=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("opens=%v want %v (glob must load sorted)", got, want)
		}
	}
}

func TestLoadFileNestedInclude(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "inner.beancount", `
2020-01-01 open Assets:Inner
`)
	writeFile(t, dir, "mid.beancount", `
include "inner.beancount"
2020-01-01 open Assets:Mid
`)
	main := writeFile(t, dir, "main.beancount", `
include "mid.beancount"
2020-01-01 open Assets:Main
`)

	dirs, diags, err := LoadFile(main)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	got := openAccounts(dirs)
	want := []string{"Assets:Inner", "Assets:Mid", "Assets:Main"}
	if len(got) != len(want) {
		t.Fatalf("opens=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("opens=%v want %v", got, want)
		}
	}
}

func TestLoadFileMissingRoot(t *testing.T) {
	_, _, err := LoadFile(filepath.Join(t.TempDir(), "nope.beancount"))
	if err == nil {
		t.Fatal("expected error for missing root file")
	}
}

func TestLoadFileFSNilUsesOS(t *testing.T) {
	dir := t.TempDir()
	main := writeFile(t, dir, "main.beancount", `
2020-01-01 open Assets:Cash
`)
	dirs, diags, err := LoadFileFS(nil, main)
	if err != nil {
		t.Fatalf("LoadFileFS(nil): %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	got := openAccounts(dirs)
	if len(got) != 1 || got[0] != "Assets:Cash" {
		t.Fatalf("opens=%v want [Assets:Cash]", got)
	}
}

func TestLoadFileFSOverlayOverridesDisk(t *testing.T) {
	dir := t.TempDir()
	main := writeFile(t, dir, "main.beancount", `
2020-01-01 open Assets:Disk
`)
	// Overlay rewrites the entry file; disk still has Assets:Disk.
	ov := filesys.NewOverlay(filesys.OS{})
	ov.Set(main, "2020-01-01 open Assets:Overlay\n")

	dirs, diags, err := LoadFileFS(ov, main)
	if err != nil {
		t.Fatalf("LoadFileFS: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	got := openAccounts(dirs)
	if len(got) != 1 || got[0] != "Assets:Overlay" {
		t.Fatalf("opens=%v want [Assets:Overlay] (overlay must win over disk)", got)
	}
}

func TestLoadFileFSOverlayIncludeNotOnDisk(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.beancount")
	incPath := filepath.Join(dir, "only-overlay.beancount")
	// Root lives on disk so Open works; include target exists only in the overlay.
	writeFile(t, dir, "main.beancount", `
include "only-overlay.beancount"
2020-01-01 open Assets:Main
`)
	ov := filesys.NewOverlay(filesys.OS{})
	ov.Set(incPath, "2020-01-01 open Assets:FromOverlay\n")

	dirs, diags, err := LoadFileFS(ov, mainPath)
	if err != nil {
		t.Fatalf("LoadFileFS: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	got := openAccounts(dirs)
	want := []string{"Assets:FromOverlay", "Assets:Main"}
	if len(got) != len(want) {
		t.Fatalf("opens=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("opens=%v want %v", got, want)
		}
	}
}

func TestLoadFileAbsoluteInclude(t *testing.T) {
	dir := t.TempDir()
	inc := writeFile(t, dir, "shared/leaf.beancount", `
2020-01-01 open Assets:Abs
`)
	// Quote the absolute path into the include so expandInclude takes the IsAbs branch.
	main := writeFile(t, dir, "main.beancount", `
include "`+inc+`"
2020-01-01 open Assets:Main
`)

	dirs, diags, err := LoadFile(main)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	got := openAccounts(dirs)
	want := []string{"Assets:Abs", "Assets:Main"}
	if len(got) != len(want) {
		t.Fatalf("opens=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("opens=%v want %v", got, want)
		}
	}
}

func TestLoadFileGlobSkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	// parts/* matches both a file and a subdirectory; dirs must be skipped.
	writeFile(t, dir, "parts/a.beancount", `
2020-01-01 open Assets:A
`)
	if err := os.MkdirAll(filepath.Join(dir, "parts", "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Marker file inside the dir must not be loaded via the parent glob.
	writeFile(t, dir, "parts/subdir/nested.beancount", `
2020-01-01 open Assets:Nested
`)
	main := writeFile(t, dir, "main.beancount", `
include "parts/*"
2020-01-01 open Assets:Main
`)

	dirs, diags, err := LoadFile(main)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	got := openAccounts(dirs)
	// Only a.beancount + main — not nested inside the matched directory.
	want := []string{"Assets:A", "Assets:Main"}
	if len(got) != len(want) {
		t.Fatalf("opens=%v want %v (glob must skip directories)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("opens=%v want %v", got, want)
		}
	}
	for _, a := range got {
		if a == "Assets:Nested" {
			t.Fatalf("loaded nested open from directory match: %v", got)
		}
	}
}

func TestLoadFileInvalidGlobPattern(t *testing.T) {
	dir := t.TempDir()
	// Unclosed character class is an invalid filepath.Glob pattern.
	main := writeFile(t, dir, "main.beancount", `
include "bad["
2020-01-01 open Assets:Cash
`)

	_, _, err := LoadFile(main)
	if err == nil {
		t.Fatal("expected error for invalid glob pattern")
	}
}
