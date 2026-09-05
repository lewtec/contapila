package engine

import (
	"sort"
	"strings"
)

// TreePathSep separates an account path from a sub-row key (commodity, file, …)
// on collapsible tree leaves. Must stay in sync with static/pnl-tree.js.
const TreePathSep = "\x1f"

// AccountTree is the sorted prefix hierarchy over leaf account paths.
// Shared by balances, net worth, P&L, and documents trees.
type AccountTree struct {
	// Names is every account path prefix, sorted (Assets, Assets:Cash, …).
	Names []string
	// HasChild[n] is true when some other name is strictly under n.
	HasChild map[string]bool
}

// NewAccountTree builds the hierarchy for the given leaf account paths.
func NewAccountTree(leaves []string) AccountTree {
	nodes := map[string]bool{}
	for _, a := range leaves {
		if a == "" {
			continue
		}
		parts := strings.Split(a, ":")
		for i := 1; i <= len(parts); i++ {
			nodes[strings.Join(parts[:i], ":")] = true
		}
	}
	names := make([]string, 0, len(nodes))
	for n := range nodes {
		names = append(names, n)
	}
	sort.Strings(names)

	hasChild := make(map[string]bool, len(names))
	for _, n := range names {
		for _, m := range names {
			if strings.HasPrefix(m, n+":") {
				hasChild[n] = true
				break
			}
		}
	}
	return AccountTree{Names: names, HasChild: hasChild}
}

// accountLeaf is the last ":" segment (Income:Ativo:BR → BR).
func accountLeaf(account string) string {
	if i := strings.LastIndex(account, ":"); i >= 0 && i+1 < len(account) {
		return account[i+1:]
	}
	return account
}

// accountDepth is hierarchy depth (number of ":" separators).
func accountDepth(account string) int {
	return strings.Count(account, ":")
}

// accountUnder reports whether leaf is prefix or a descendant of prefix.
func accountUnder(leaf, prefix string) bool {
	return leaf == prefix || strings.HasPrefix(leaf, prefix+":")
}
