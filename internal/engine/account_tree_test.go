package engine

import "testing"

func TestNewAccountTree(t *testing.T) {
	tree := NewAccountTree([]string{"Assets:Cash", "Assets:Bank:Checking", "Expenses:Food"})
	// prefixes + leaves
	wantNames := map[string]bool{
		"Assets": true, "Assets:Cash": true, "Assets:Bank": true, "Assets:Bank:Checking": true,
		"Expenses": true, "Expenses:Food": true,
	}
	if len(tree.Names) != len(wantNames) {
		t.Fatalf("names=%v", tree.Names)
	}
	for _, n := range tree.Names {
		if !wantNames[n] {
			t.Fatalf("unexpected %s", n)
		}
	}
	if !tree.HasChild["Assets"] || !tree.HasChild["Assets:Bank"] {
		t.Fatalf("HasChild=%v", tree.HasChild)
	}
	if tree.HasChild["Assets:Cash"] || tree.HasChild["Expenses:Food"] {
		t.Fatalf("leaves should not have children: %v", tree.HasChild)
	}
	if accountLeaf("Assets:Bank:Checking") != "Checking" {
		t.Fatal(accountLeaf("Assets:Bank:Checking"))
	}
	if !accountUnder("Assets:Cash", "Assets") || accountUnder("Expenses:Food", "Assets") {
		t.Fatal("accountUnder")
	}
}
