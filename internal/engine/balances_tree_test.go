package engine

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBalancesTreeLeafNames(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "example")
	p, pdb, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(p, pdb, "personal")
	if err != nil {
		t.Fatal(err)
	}
	tree := l.BalancesTree(AsOfLatest)
	if len(tree) == 0 {
		t.Fatal("empty tree")
	}
	var sawAssets bool
	for _, ln := range tree {
		if strings.Contains(ln.Name, ":") {
			t.Fatalf("name should be leaf segment, got %q", ln.Name)
		}
		if ln.Account == "Assets" {
			sawAssets = true
			if !ln.IsRollup {
				t.Fatal("Assets should be rollup")
			}
		}
	}
	if !sawAssets {
		t.Fatal("missing Assets root")
	}
}
