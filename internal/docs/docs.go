// Package docs expands per-ledger <ledger>/docs/by-account trees into document directives.
package docs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
)

// ByAccountDir is the account document folder under each ledger's docs/.
const ByAccountDir = "by-account"

// Leading calendar date on a by-account filename.
// Groups: 1=yyyy (required), 2=mm (optional), 3=dd (optional). Omitted → 01.
// Must be followed by a non-digit or end of name (so 5/7-digit runs fail).
var datePrefix = regexp.MustCompile(`^(\d{4})(\d{2})?(\d{2})?(?:\D|$)`)

// ScanByAccount walks <root>/<ledger>/docs/by-account and synthesizes document directives.
// Account path is directory segments joined with ':' under by-account;
// file names must start with a calendar date prefix: yyyy, yyyymm, or yyyymmdd
// (omitted month/day default to 01). Under an account directory, any file that
// does not parse is skipped and reported as an error diagnostic (project-relative
// path). Files directly under by-account/ (no account segment) are ignored.
func ScanByAccount(projectRoot, ledger string) ([]ast.Document, diag.List, error) {
	if ledger == "" {
		return nil, nil, nil
	}
	root := filepath.Join(projectRoot, ledger, "docs", ByAccountDir)
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, nil
	}

	var out []ast.Document
	var diags diag.List
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		dir := filepath.ToSlash(filepath.Dir(rel))
		if dir == "." {
			// Loose file under by-account/ — no account path.
			return nil
		}
		projRel := filepath.ToSlash(filepath.Join(ledger, "docs", ByAccountDir, rel))
		base := d.Name()
		dt, perr := parseFilenameDate(base)
		if perr != "" {
			diags.Error(projRel, 0, perr)
			return nil
		}
		account := strings.ReplaceAll(dir, "/", ":")
		out = append(out, ast.Document{
			Meta:      ast.Meta{Date: dt, File: "docs.gen"},
			Account:   account,
			Path:      projRel,
			Synthetic: true,
		})
		return nil
	})
	if err != nil {
		return nil, diags, err
	}
	sortDocuments(out)
	return out, diags, nil
}

// parseFilenameDate reads capture groups yyyy / optional mm / optional dd.
// Omitted month or day defaults to 01. On failure returns a non-empty message.
func parseFilenameDate(base string) (time.Time, string) {
	m := datePrefix.FindStringSubmatch(base)
	if m == nil {
		return time.Time{}, fmt.Sprintf("filename %q must start with yyyy, yyyymm, or yyyymmdd", base)
	}
	year, month, day := m[1], m[2], m[3]
	if month == "" {
		month = "01"
	}
	if day == "" {
		day = "01"
	}
	prefix := year + month + day
	// Show only what the user wrote when reporting calendar errors.
	written := year
	if m[2] != "" {
		written += m[2]
	}
	if m[3] != "" {
		written += m[3]
	}
	dt, err := time.ParseInLocation("20060102", prefix, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Sprintf("filename date prefix %q is not a valid calendar date", written)
	}
	// Reject Go normalization (e.g. 20240230 → March 1).
	if dt.Format("20060102") != prefix {
		return time.Time{}, fmt.Sprintf("filename date prefix %q is not a valid calendar date", written)
	}
	return dt, ""
}

// sortDocuments orders newest first, then account, then path (stable).
func sortDocuments(docs []ast.Document) {
	sort.SliceStable(docs, func(i, j int) bool {
		if !docs[i].Date.Equal(docs[j].Date) {
			return docs[i].Date.After(docs[j].Date)
		}
		if docs[i].Account != docs[j].Account {
			return docs[i].Account < docs[j].Account
		}
		return docs[i].Path < docs[j].Path
	})
}

// Merge combines ledger document directives with synthetic docs.
// Prefer explicit (non-synthetic) when the same Path appears twice.
func Merge(fromLedger, synthetic []ast.Document) []ast.Document {
	byPath := map[string]ast.Document{}
	order := make([]string, 0)
	add := func(d ast.Document) {
		if d.Path == "" {
			return
		}
		key := filepath.ToSlash(d.Path)
		if prev, ok := byPath[key]; ok {
			if prev.Synthetic && !d.Synthetic {
				byPath[key] = d
			}
			return
		}
		byPath[key] = d
		order = append(order, key)
	}
	for _, d := range fromLedger {
		add(d)
	}
	for _, d := range synthetic {
		add(d)
	}
	out := make([]ast.Document, 0, len(order))
	for _, k := range order {
		out = append(out, byPath[k])
	}
	sortDocuments(out)
	return out
}

// ForAccount filters documents linked to account (exact match).
func ForAccount(docs []ast.Document, account string) []ast.Document {
	var out []ast.Document
	for _, d := range docs {
		if d.Account == account {
			out = append(out, d)
		}
	}
	return out
}

// IsLedgerDocPath reports whether rel is under <ledger>/docs/ (safe to serve).
// After slash-normalize and slash trim, rel must be <ledger>/docs/<…> with no
// empty segments or ".." components (checked before Clean so they are not
// folded away). The shape is verified on path.Clean form.
func IsLedgerDocPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	rel = strings.Trim(rel, "/")
	if rel == "" || rel == "." {
		return false
	}
	for _, p := range strings.Split(rel, "/") {
		if p == "" || p == ".." {
			return false
		}
	}
	cleaned := path.Clean(rel)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return false
	}
	parts := strings.Split(cleaned, "/")
	if len(parts) < 3 || parts[1] != "docs" {
		return false
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return false
		}
	}
	return true
}
