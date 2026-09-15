package engine

import (
	"math/big"
	"sort"
	"time"
)

// NetWorthTreeLine is one hierarchical net-worth row (value in op currency).
// Name is the last path segment (Fava-style); Account is the full path for links.
// Path is the tree identity for collapse (account, or account+\x1f+commodity).
type NetWorthTreeLine struct {
	Account   string
	Path      string // collapse key
	Name      string
	Depth     int
	Units     *big.Rat // nil when multi-commodity rollup / parent
	Commodity string
	Value     *big.Rat // signed, operating currency
	IsRollup  bool
	Unpriced  bool // true when any rolled-up line lacked a market price
}

// NetWorthTree returns Assets/Liabilities as a collapsible account tree.
func (l *Ledger) NetWorthTree(asOf time.Time) ([]NetWorthTreeLine, *big.Rat, error) {
	lines, total, err := l.NetWorth(asOf)
	if err != nil {
		return nil, nil, err
	}
	return netWorthTreeFromLines(lines), total, nil
}

func netWorthTreeFromLines(lines []NetWorthLine) []NetWorthTreeLine {
	if len(lines) == 0 {
		return nil
	}

	type agg struct {
		value    *big.Rat
		unpriced bool
		comms    map[string]*big.Rat // commodity -> units
		// per-commodity value for multi-ccy leaves
		commVal      map[string]*big.Rat
		commUnpriced map[string]bool
	}
	byAcct := map[string]*agg{}
	for _, ln := range lines {
		a := byAcct[ln.Account]
		if a == nil {
			a = &agg{
				value:        big.NewRat(0, 1),
				comms:        map[string]*big.Rat{},
				commVal:      map[string]*big.Rat{},
				commUnpriced: map[string]bool{},
			}
			byAcct[ln.Account] = a
		}
		a.value.Add(a.value, ln.Value)
		if ln.Unpriced {
			a.unpriced = true
			a.commUnpriced[ln.Commodity] = true
		}
		if ln.Units != nil {
			if a.comms[ln.Commodity] == nil {
				a.comms[ln.Commodity] = big.NewRat(0, 1)
			}
			a.comms[ln.Commodity].Add(a.comms[ln.Commodity], ln.Units)
		}
		if a.commVal[ln.Commodity] == nil {
			a.commVal[ln.Commodity] = big.NewRat(0, 1)
		}
		a.commVal[ln.Commodity].Add(a.commVal[ln.Commodity], ln.Value)
	}

	leaves := make([]string, 0, len(byAcct))
	for a := range byAcct {
		leaves = append(leaves, a)
	}
	tree := NewAccountTree(leaves)

	rollupVal := map[string]*big.Rat{}
	rollupUnpriced := map[string]bool{}
	for _, n := range tree.Names {
		tot := big.NewRat(0, 1)
		unpriced := false
		for a, ag := range byAcct {
			if accountUnder(a, n) {
				tot.Add(tot, ag.value)
				if ag.unpriced {
					unpriced = true
				}
			}
		}
		if tot.Sign() != 0 {
			rollupVal[n] = tot
			rollupUnpriced[n] = unpriced
		}
	}

	out := make([]NetWorthTreeLine, 0, len(tree.Names)+8)
	for _, n := range tree.Names {
		val := rollupVal[n]
		if val == nil {
			continue
		}
		child := tree.HasChild[n]
		row := NetWorthTreeLine{
			Account:  n,
			Path:     n,
			Name:     accountLeaf(n),
			Depth:    accountDepth(n),
			Value:    new(big.Rat).Set(val),
			IsRollup: child,
			Unpriced: rollupUnpriced[n],
		}
		ag := byAcct[n]
		if !child && ag != nil && len(ag.comms) == 1 {
			for c, u := range ag.comms {
				row.Commodity = c
				row.Units = new(big.Rat).Set(u)
			}
		}
		// Multi-commodity leaf account is a rollup of commodity sub-rows
		if !child && ag != nil && len(ag.comms) > 1 {
			row.IsRollup = true
		}
		out = append(out, row)

		if !child && ag != nil && len(ag.comms) > 1 {
			var cs []string
			for c := range ag.comms {
				cs = append(cs, c)
			}
			sort.Strings(cs)
			for _, c := range cs {
				out = append(out, NetWorthTreeLine{
					Account:   n,
					Path:      n + TreePathSep + c,
					Name:      c,
					Depth:     accountDepth(n) + 1,
					Units:     new(big.Rat).Set(ag.comms[c]),
					Commodity: c,
					Value:     new(big.Rat).Set(ag.commVal[c]),
					IsRollup:  false,
					Unpriced:  ag.commUnpriced[c],
				})
			}
		}
	}
	return out
}
