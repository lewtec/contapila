package engine

import (
	"math/big"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBalancesTreeLeafNames(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "example")
	p, pdb, _, err := OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(t.Context(), p, pdb, "personal")
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
	direct := tree
	via := BalancesTreeFrom(l.BalancesAsOf(AsOfLatest))
	require.Len(t, via, len(direct))
	for i := range direct {
		assert.Equal(t, direct[i].Account, via[i].Account, "line %d account", i)
		assert.Equal(t, direct[i].Commodity, via[i].Commodity, "line %d commodity", i)
		assert.Equal(t, direct[i].IsRollup, via[i].IsRollup, "line %d rollup", i)
		assert.True(t, sameRat(direct[i].Amount, via[i].Amount), "line %d amount", i)
	}
}

func TestSelectAccounts(t *testing.T) {
	brl := big.NewRat(10, 1)
	usd := big.NewRat(3, 1)
	bals := map[string]map[string]*big.Rat{
		"Assets:Cash":        {"BRL": brl},
		"Assets:Cash:Wallet": {"BRL": brl},
		"Assets:Bank":        {"USD": usd},
		"Expenses:Food":      {"BRL": brl},
	}
	got := SelectAccounts(bals, nil)
	require.Len(t, got, 4)
	assert.Same(t, brl, got["Assets:Cash"]["BRL"])

	empty := SelectAccounts(bals, []*regexp.Regexp{})
	assert.Same(t, brl, empty["Assets:Cash"]["BRL"])

	none := SelectAccounts(bals, []*regexp.Regexp{nil})
	assert.Empty(t, none)

	assets := SelectAccounts(bals, patterns(t, "Assets"))
	assert.NotContains(t, assets, "Expenses:Food")
	assert.Contains(t, assets, "Assets:Cash")
	assert.Contains(t, assets, "Assets:Cash:Wallet")
	assert.Contains(t, assets, "Assets:Bank")

	cash := SelectAccounts(bals, patterns(t, "Cash"))
	assert.Contains(t, cash, "Assets:Cash")
	assert.Contains(t, cash, "Assets:Cash:Wallet")
	assert.NotContains(t, cash, "Assets:Bank")
	assert.NotContains(t, cash, "Expenses:Food")

	exact := SelectAccounts(bals, patterns(t, "^Assets:Cash$"))
	assert.Contains(t, exact, "Assets:Cash")
	assert.NotContains(t, exact, "Assets:Cash:Wallet")

	food := SelectAccounts(bals, patterns(t, "^Expenses", "Food$"))
	assert.Len(t, food, 1)
	assert.Contains(t, food, "Expenses:Food")

	one := SelectAccounts(bals, patterns(t, "^Assets:Cash$", "^Expenses:Food$"))
	assert.Len(t, one, 2)

	filtered := BalancesTreeFrom(SelectAccounts(bals, patterns(t, "^Assets:Cash$")))
	var accounts []string
	for _, line := range filtered {
		accounts = append(accounts, line.Account)
	}
	assert.NotContains(t, accounts, "Expenses:Food")
	assert.NotContains(t, accounts, "Assets:Cash:Wallet")
	assert.Contains(t, accounts, "Assets")
	assert.Contains(t, accounts, "Assets:Cash")
}

func patterns(t *testing.T, exprs ...string) []*regexp.Regexp {
	t.Helper()
	out := make([]*regexp.Regexp, len(exprs))
	for i, expr := range exprs {
		re, err := regexp.Compile(expr)
		require.NoError(t, err)
		out[i] = re
	}
	return out
}

func sameRat(a, b *big.Rat) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Cmp(b) == 0
}
