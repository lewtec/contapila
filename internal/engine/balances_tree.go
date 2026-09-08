package engine

import (
	"math/big"
	"sort"
	"time"
)

// BalanceTreeLine is one hierarchical balances row (native units per commodity).
// Name is the last path segment; Account is the full path for links.
// Path is the tree identity for collapse (account, or account+\x1f+commodity).
type BalanceTreeLine struct {
	Account   string
	Path      string
	Name      string
	Depth     int
	Amount    *big.Rat // nil for pure structural / multi-ccy parents
	Commodity string
	IsRollup  bool
}

// BalancesTree returns all non-zero balances as a Fava-style account tree.
func (l *Ledger) BalancesTree(asOf time.Time) []BalanceTreeLine {
	return balancesTreeFromMap(l.BalancesAsOf(asOf))
}

func balancesTreeFromMap(bals map[string]map[string]*big.Rat) []BalanceTreeLine {
	// account -> commodity -> units (skip zeros)
	byAcct := map[string]map[string]*big.Rat{}
	for acct, byComm := range bals {
		for comm, n := range byComm {
			if n == nil || n.Sign() == 0 {
				continue
			}
			if byAcct[acct] == nil {
				byAcct[acct] = map[string]*big.Rat{}
			}
			byAcct[acct][comm] = new(big.Rat).Set(n)
		}
	}
	if len(byAcct) == 0 {
		return nil
	}

	leaves := make([]string, 0, len(byAcct))
	for a := range byAcct {
		leaves = append(leaves, a)
	}
	tree := NewAccountTree(leaves)

	// A prefix node is "present" if any leaf account is under it.
	hasBalanceUnder := map[string]bool{}
	for _, n := range tree.Names {
		for a := range byAcct {
			if accountUnder(a, n) {
				hasBalanceUnder[n] = true
				break
			}
		}
	}

	out := make([]BalanceTreeLine, 0, len(tree.Names)+8)
	for _, n := range tree.Names {
		if !hasBalanceUnder[n] {
			continue
		}
		ag := byAcct[n]
		child := tree.HasChild[n]
		row := BalanceTreeLine{
			Account:  n,
			Path:     n,
			Name:     accountLeaf(n),
			Depth:    accountDepth(n),
			IsRollup: child,
		}
		if !child && ag != nil && len(ag) == 1 {
			for c, u := range ag {
				row.Commodity = c
				row.Amount = new(big.Rat).Set(u)
			}
		}
		if !child && ag != nil && len(ag) > 1 {
			row.IsRollup = true
		}
		// Structural parents (no own balances) are always rollups
		if child {
			row.IsRollup = true
		}
		out = append(out, row)

		if !child && ag != nil && len(ag) > 1 {
			var cs []string
			for c := range ag {
				cs = append(cs, c)
			}
			sort.Strings(cs)
			for _, c := range cs {
				out = append(out, BalanceTreeLine{
					Account:   n,
					Path:      n + TreePathSep + c,
					Name:      c,
					Depth:     accountDepth(n) + 1,
					Amount:    new(big.Rat).Set(ag[c]),
					Commodity: c,
					IsRollup:  false,
				})
			}
		}
	}
	return out
}
