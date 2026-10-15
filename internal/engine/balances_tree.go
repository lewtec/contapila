package engine

import (
	"math/big"
	"regexp"
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
	return BalancesTreeFrom(l.BalancesAsOf(asOf))
}

// BalancesTreeFrom is the balances tree for an already loaded balance map.
func BalancesTreeFrom(bals map[string]map[string]*big.Rat) []BalanceTreeLine {
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

	out := make([]BalanceTreeLine, 0, len(tree.Names)+8)
	for _, n := range tree.Names {
		ag := byAcct[n]
		child := tree.HasChild[n]
		row := BalanceTreeLine{
			Account:  n,
			Path:     n,
			Name:     AccountLeaf(n),
			Depth:    AccountDepth(n),
			IsRollup: child || len(ag) > 1,
		}
		if !child && len(ag) == 1 {
			for c, u := range ag {
				row.Commodity = c
				row.Amount = new(big.Rat).Set(u)
			}
		}
		out = append(out, row)

		if !child && len(ag) > 1 {
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
					Depth:     AccountDepth(n) + 1,
					Amount:    new(big.Rat).Set(ag[c]),
					Commodity: c,
					IsRollup:  false,
				})
			}
		}
	}
	return out
}

// SelectAccounts keeps accounts that match any filter.
// Each filter is a RE2 pattern. MatchString is unanchored unless the pattern uses ^ or $.
// An empty filters list returns bals unchanged. A nil filter does not match.
func SelectAccounts(bals map[string]map[string]*big.Rat, filters []*regexp.Regexp) map[string]map[string]*big.Rat {
	if len(filters) == 0 {
		return bals
	}
	out := make(map[string]map[string]*big.Rat)
	for account, byCommodity := range bals {
		if accountMatches(account, filters) {
			out[account] = byCommodity
		}
	}
	return out
}

func accountMatches(account string, filters []*regexp.Regexp) bool {
	for _, filter := range filters {
		if filter != nil && filter.MatchString(account) {
			return true
		}
	}
	return false
}
