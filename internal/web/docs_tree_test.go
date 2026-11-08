package web

import (
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
)

func TestDocumentTreeRowsHierarchy(t *testing.T) {
	docs := []ast.Document{
		{Meta: ast.Meta{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)}, Account: "Assets:Cash", Path: "personal/docs/by-account/Assets/Cash/20240102_a.txt", Synthetic: true},
		{Meta: ast.Meta{Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)}, Account: "Assets:Cash", Path: "personal/docs/by-account/Assets/Cash/20240301_b.txt", Synthetic: true},
		{Meta: ast.Meta{Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)}, Account: "Assets:Bank:Checking", Path: "personal/docs/by-account/Assets/Bank/Checking/20240201_c.txt", Synthetic: true},
	}
	rows := documentTreeRows(docs)
	if len(rows) < 4 {
		t.Fatalf("rows=%d %+v", len(rows), rows)
	}
	// Expect Assets rollup, then Bank or Cash branches
	var hasAssets, hasMultiCashDocs, hasSingleChecking bool
	for _, r := range rows {
		if r.Account == "Assets" && r.IsRollup {
			hasAssets = true
		}
		if r.IsDoc && r.Account == "Assets:Cash" {
			hasMultiCashDocs = true
		}
		if r.Account == "Assets:Bank:Checking" && !r.IsRollup && !r.IsDoc && r.FileName != "" {
			hasSingleChecking = true
		}
	}
	if !hasAssets {
		t.Fatalf("missing Assets rollup: %+v", rows)
	}
	if !hasMultiCashDocs {
		t.Fatalf("expected Cash multi-doc children: %+v", rows)
	}
	if !hasSingleChecking {
		t.Fatalf("expected single-doc Checking leaf: %+v", rows)
	}
}
