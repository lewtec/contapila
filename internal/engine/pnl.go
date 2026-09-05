package engine

import (
	"math/big"
	"time"
)

// PnLLine is one row of a hierarchical income statement.
// Amounts keep Beancount signs (income typically −, expenses typically +)
// and are converted to operating currency when set.
type PnLLine struct {
	Account   string // full account path (for links / identity)
	Name      string // last segment only (Fava-style tree label)
	Depth     int    // colons in account name (Income=0, Income:X=1, …)
	Amount    *big.Rat
	IsRollup  bool // has child accounts in this statement
	Commodity string
}

// PnLTree builds Fava-style account trees for income and expenses over [from,to].
func (l *Ledger) PnLTree(from, to time.Time) (income, expenses []PnLLine) {
	raw := l.PnL(from, to)
	return l.pnlTreeSection(raw.Income), l.pnlTreeSection(raw.Expenses)
}

func (l *Ledger) pnlTreeSection(m map[string]map[string]*big.Rat) []PnLLine {
	flat := map[string]*big.Rat{}
	for acct, byComm := range m {
		sum := big.NewRat(0, 1)
		for comm, n := range byComm {
			if n == nil || n.Sign() == 0 {
				continue
			}
			v, _ := l.marketConvert(comm, n, AsOfLatest, false)
			sum.Add(sum, v)
		}
		if sum.Sign() != 0 {
			flat[acct] = sum
		}
	}
	if len(flat) == 0 {
		return nil
	}

	leaves := make([]string, 0, len(flat))
	for a := range flat {
		leaves = append(leaves, a)
	}
	tree := NewAccountTree(leaves)

	rollup := map[string]*big.Rat{}
	for _, n := range tree.Names {
		tot := big.NewRat(0, 1)
		for a, v := range flat {
			if accountUnder(a, n) {
				tot.Add(tot, v)
			}
		}
		if tot.Sign() != 0 {
			rollup[n] = tot
		}
	}

	out := make([]PnLLine, 0, len(tree.Names))
	for _, n := range tree.Names {
		amt := rollup[n]
		if amt == nil {
			continue
		}
		out = append(out, PnLLine{
			Account:   n,
			Name:      accountLeaf(n),
			Depth:     accountDepth(n),
			Amount:    new(big.Rat).Set(amt),
			IsRollup:  tree.HasChild[n],
			Commodity: l.OpCurrency,
		})
	}
	return out
}
