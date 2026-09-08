package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/contapila-go/internal/filesys"
)

func TestDiscovery(t *testing.T) {
	tmp := t.TempDir()

	// Setup layout
	// /tmp/root/
	//   contapila.cue
	//   personal/main.beancount
	//   business/main.beancount
	//   other/not-a-ledger.txt

	root := filepath.Join(tmp, "project")
	err := os.MkdirAll(filepath.Join(root, "personal"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(filepath.Join(root, "business"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(filepath.Join(root, "other"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(root, "contapila.cue"), []byte("{}"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(root, "personal", "main.beancount"), []byte(""), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(root, "business", "main.beancount"), []byte(""), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(root, "other", "not-a-ledger.txt"), []byte(""), 0644)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("findRoot", func(t *testing.T) {
		found, err := findRoot(filesys.OS{}, filepath.Join(root, "personal"))
		if err != nil {
			t.Fatalf("failed to find root: %v", err)
		}
		if found != root {
			t.Errorf("expected root %s, got %s", root, found)
		}
	})

	t.Run("discoverLedgers", func(t *testing.T) {
		ledgers, err := discoverLedgers(filesys.OS{}, root)
		if err != nil {
			t.Fatalf("failed to discover ledgers: %v", err)
		}
		if len(ledgers) != 2 {
			t.Errorf("expected 2 ledgers, got %d", len(ledgers))
		}
		names := make(map[string]bool)
		for _, l := range ledgers {
			names[l.Name] = true
		}
		if !names["personal"] || !names["business"] {
			t.Errorf("missing expected ledgers personal/business: %v", names)
		}
		if names["other"] {
			t.Errorf("should not have discovered 'other' as a ledger")
		}
	})

	t.Run("OpenProject", func(t *testing.T) {
		p, err := OpenProject(filepath.Join(root, "personal"))
		if err != nil {
			t.Fatalf("failed to open project: %v", err)
		}
		if p.Root != root {
			t.Errorf("expected root %s, got %s", root, p.Root)
		}
		if len(p.Ledgers) != 2 {
			t.Errorf("expected 2 ledgers, got %d", len(p.Ledgers))
		}
		if !p.PricesMissing {
			t.Errorf("expected prices missing")
		}
	})

	t.Run("not a project", func(t *testing.T) {
		_, err := findRoot(filesys.OS{}, tmp)
		if err == nil {
			t.Errorf("expected error searching for root in non-project dir")
		}
	})
}
