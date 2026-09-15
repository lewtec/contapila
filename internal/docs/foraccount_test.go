package docs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
)

func TestForAccount(t *testing.T) {
	d1 := ast.Document{
		Meta:    ast.Meta{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		Account: "Assets:Cash",
		Path:    "personal/docs/by-account/Assets/Cash/20240101_a.txt",
	}
	d2 := ast.Document{
		Meta:    ast.Meta{Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
		Account: "Assets:Cash",
		Path:    "personal/docs/by-account/Assets/Cash/20240201_b.txt",
	}
	d3 := ast.Document{
		Meta:    ast.Meta{Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
		Account: "Assets:BR:Cash",
		Path:    "personal/docs/by-account/Assets/BR/Cash/20240301_c.txt",
	}
	all := []ast.Document{d1, d2, d3}

	t.Run("exact match", func(t *testing.T) {
		got := ForAccount(all, "Assets:BR:Cash")
		if len(got) != 1 || got[0].Path != d3.Path {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("multiple docs same account", func(t *testing.T) {
		got := ForAccount(all, "Assets:Cash")
		if len(got) != 2 {
			t.Fatalf("got %d: %+v", len(got), got)
		}
		if got[0].Path != d1.Path || got[1].Path != d2.Path {
			t.Fatalf("order/paths: %+v", got)
		}
	})

	t.Run("no match", func(t *testing.T) {
		got := ForAccount(all, "Liabilities:Card")
		if len(got) != 0 {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("empty docs", func(t *testing.T) {
		got := ForAccount(nil, "Assets:Cash")
		if got != nil && len(got) != 0 {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("exact match not prefix", func(t *testing.T) {
		// DocumentsForAccount / ForAccount are exact account name, not subaccounts.
		got := ForAccount(all, "Assets")
		if len(got) != 0 {
			t.Fatalf("prefix must not match: %+v", got)
		}
		got = ForAccount(all, "Assets:Cash:Sub")
		if len(got) != 0 {
			t.Fatalf("child name must not match parent docs: %+v", got)
		}
	})

	t.Run("empty account filter", func(t *testing.T) {
		withEmpty := append(all, ast.Document{
			Account: "",
			Path:    "personal/docs/orphan.txt",
		})
		got := ForAccount(withEmpty, "")
		if len(got) != 1 || got[0].Path != "personal/docs/orphan.txt" {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestScanByAccount_edges(t *testing.T) {
	t.Run("missing by-account dir", func(t *testing.T) {
		root := t.TempDir()
		got, err := ScanByAccount(root, "personal")
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("empty ledger name", func(t *testing.T) {
		got, err := ScanByAccount(t.TempDir(), "")
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("file directly under by-account skipped", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "personal", "docs", "by-account")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "20240101_loose.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := ScanByAccount(root, "personal")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("root-level file should be skipped: %+v", got)
		}
	})

	t.Run("nested multi-segment account", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "personal", "docs", "by-account", "Assets", "BR", "Broker", "Cash")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "20240515_note.pdf"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		// No date prefix — ignored.
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := ScanByAccount(root, "personal")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d: %+v", len(got), got)
		}
		if got[0].Account != "Assets:BR:Broker:Cash" {
			t.Fatalf("account=%q", got[0].Account)
		}
		want := "personal/docs/by-account/Assets/BR/Broker/Cash/20240515_note.pdf"
		if got[0].Path != want {
			t.Fatalf("path=%q want %q", got[0].Path, want)
		}
		if !got[0].Date.Equal(time.Date(2024, 5, 15, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("date=%v", got[0].Date)
		}
	})
}
