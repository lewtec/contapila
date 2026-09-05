package engine

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNetWorthTreeLeafNames(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "example")
	p, pdb, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(p, pdb, "personal")
	if err != nil {
		t.Fatal(err)
	}
	tree, total, err := l.NetWorthTree(AsOfLatest)
	if err != nil {
		t.Fatal(err)
	}
	if total.Sign() == 0 {
		t.Fatal("zero NW")
	}
	var sawAssets, sawLeaf bool
	for _, ln := range tree {
		if strings.Contains(ln.Name, ":") {
			t.Fatalf("name should be leaf segment, got %q for %s", ln.Name, ln.Account)
		}
		if ln.Account == "Assets" {
			sawAssets = true
			if !ln.IsRollup {
				t.Fatal("Assets should be rollup")
			}
		}
		if ln.Name == "ContaCorrente" && strings.Contains(ln.Account, "Alfa") {
			sawLeaf = true
		}
	}
	if !sawAssets || !sawLeaf {
		t.Fatalf("assets=%v leaf=%v n=%d", sawAssets, sawLeaf, len(tree))
	}
}
