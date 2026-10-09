package main

import (
	"os"
	"testing"

	"github.com/lewtec/lewkit/x/cmd"
	lewtest "github.com/lewtec/lewkit/x/test"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBalancesUnfilteredExample(t *testing.T) {
	dir := exampleDir(t)
	t.Chdir(dir)
	lewtest.DiscardSlog(t)

	all := runBalances(t)
	assert.Contains(t, all, "LEDGER")
	assert.Contains(t, all, "ACCOUNT")
	assert.Contains(t, all, "personal")
	assert.Contains(t, all, "acme")
	assert.Contains(t, all, "ong")
	assert.Contains(t, all, "smuggle")
	assert.Contains(t, all, "Assets:Cash:Carteira")
	assert.Contains(t, all, "268209.220000")
	assert.NotContains(t, all, "== personal balances ==")

	personal := runBalances(t, "personal")
	assert.Contains(t, personal, "== personal balances ==")
	assert.Contains(t, personal, "ContaCorrente")
	assert.Contains(t, personal, "268209.2200")
	assert.Contains(t, personal, "B3_PETR4")
	assert.NotContains(t, personal, "LEDGER")
	assert.Equal(t, personal, runBalances(t, "--ledger", "personal"))
	assert.Equal(t, personal, runBalances(t, "personal", "--ledger", "personal"))
}

func TestBalancesAccountFilter(t *testing.T) {
	t.Chdir(exampleDir(t))
	lewtest.DiscardSlog(t)

	one := runBalances(t, "personal", "--account", "Assets:Cash:Carteira")
	assert.Contains(t, one, "== personal balances ==")
	assert.Contains(t, one, "Carteira")
	assert.Contains(t, one, "245.0000")
	assert.Contains(t, one, "Assets")
	assert.NotContains(t, one, "ContaCorrente")
	assert.NotContains(t, one, "Poupanca")

	alfa := runBalances(t, "personal", "--account", "Assets:BR:Alfa")
	assert.Contains(t, alfa, "ContaCorrente")
	assert.Contains(t, alfa, "Poupanca")
	assert.NotContains(t, alfa, "Carteira")
	assert.NotContains(t, alfa, "B3_PETR4")

	middle := runBalances(t, "personal", "--account", "Alfa")
	assert.Contains(t, middle, "ContaCorrente")
	assert.Contains(t, middle, "Poupanca")
	assert.NotContains(t, middle, "Carteira")

	end := runBalances(t, "personal", "--account", "Carteira$")
	assert.Contains(t, end, "Carteira")
	assert.Contains(t, end, "245.0000")
	assert.NotContains(t, end, "ContaCorrente")

	exact := runBalances(t, "personal", "--account", "^Assets:Cash:Carteira$")
	assert.Contains(t, exact, "Carteira")
	assert.Contains(t, exact, "245.0000")
	assert.NotContains(t, exact, "ContaCorrente")

	anchoredMiss := runBalances(t, "personal", "--account", "Cash$")
	assert.Equal(t, "== personal balances ==\n", anchoredMiss)

	dot := runBalances(t, "personal", "--account", "Assets.Cash.Carteira")
	assert.Contains(t, dot, "Carteira")
	assert.Contains(t, dot, "245.0000")
	assert.NotContains(t, dot, "ContaCorrente")

	both := runBalances(t, "--ledger", "personal", "--account", "Assets:Cash:Carteira", "--account", "Assets:BR:Alfa:Poupanca")
	assert.Contains(t, both, "Carteira")
	assert.Contains(t, both, "Poupanca")
	assert.NotContains(t, both, "ContaCorrente")

	flat := runBalances(t, "--account", "Assets:Cash:Carteira")
	assert.Contains(t, flat, "LEDGER")
	assert.Contains(t, flat, "Assets:Cash:Carteira")
	assert.NotContains(t, flat, "Poupanca")
	assert.NotContains(t, flat, "== personal balances ==")

	none := runBalances(t, "--ledger", "personal", "--account", "No:Such")
	assert.Equal(t, "== personal balances ==\n", none)
}

func TestBalancesLedgerFlag(t *testing.T) {
	dir := exampleDir(t)
	t.Chdir(dir)
	lewtest.DiscardSlog(t)

	err := cmd.ParseErr[cmd.App[root]](t, "balances", "--ledger", "nope")
	require.ErrorIs(t, err, cmd.ErrInvalidArgument)
	assert.Contains(t, err.Error(), "acme, ong, personal, smuggle")

	app := cmd.ParseOK[cmd.App[root]](t, "balances", "personal", "--ledger", "acme")
	require.ErrorIs(t, app.Run(t.Context()), ErrLedgerConflict)

	unknown := cmd.ParseOK[cmd.App[root]](t, "balances", "nope")
	require.ErrorIs(t, unknown.Run(t.Context()), engine.ErrUnknownLedger)

	empty := cmd.ParseErr[cmd.App[root]](t, "balances", "--account", "")
	require.ErrorIs(t, empty, cmd.ErrInvalidArgument)
	assert.Contains(t, empty.Error(), "empty pattern")

	bad := cmd.ParseErr[cmd.App[root]](t, "balances", "--account", "[")
	require.ErrorIs(t, bad, cmd.ErrInvalidArgument)
}

func TestBalancesHelpLedgerChoices(t *testing.T) {
	dir := exampleDir(t)
	t.Chdir(dir)
	lewtest.DiscardSlog(t)
	out := balancesHelp(t, "balances", "--help")
	assert.Contains(t, out, "--account")
	assert.Contains(t, out, "unanchored")
	assert.Contains(t, out, "--ledger")
	assert.Contains(t, out, "choices: acme, ong, personal, smuggle")

	t.Chdir(t.TempDir())
	out = balancesHelp(t, "balances", "--help")
	assert.Contains(t, out, "--ledger")
	assert.NotContains(t, out, "choices:")
}

func TestBalancesHelpDirectoryFlag(t *testing.T) {
	dir := exampleDir(t)
	t.Chdir(t.TempDir())
	t.Setenv("CONTAPILA_DIRECTORY", t.TempDir())
	withArgs(t, "contapila", "-C", dir, "balances", "--help")
	lewtest.DiscardSlog(t)
	out := balancesHelp(t, "-C", dir, "balances", "--help")
	assert.Contains(t, out, "choices: acme, ong, personal, smuggle")
}

func TestBalancesHelpDirectoryEnv(t *testing.T) {
	dir := exampleDir(t)
	t.Chdir(t.TempDir())
	t.Setenv("CONTAPILA_DIRECTORY", dir)
	withArgs(t, "contapila", "balances", "--help")
	lewtest.DiscardSlog(t)
	out := balancesHelp(t, "balances", "--help")
	assert.Contains(t, out, "choices: acme, ong, personal, smuggle")
}

func TestDirectoryFromArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
		ok   bool
	}{
		{name: "short", args: []string{"-C", "proj", "balances"}, want: "proj", ok: true},
		{name: "long", args: []string{"balances", "--directory", "proj"}, want: "proj", ok: true},
		{name: "long equals", args: []string{"--directory=proj", "balances"}, want: "proj", ok: true},
		{name: "attached", args: []string{"-Cproj", "balances"}, want: "proj", ok: true},
		{name: "short equals", args: []string{"-C=proj"}, want: "proj", ok: true},
		{name: "last wins", args: []string{"-C", "first", "--directory", "second"}, want: "second", ok: true},
		{name: "absent", args: []string{"balances", "--help"}, ok: false},
		{name: "missing value", args: []string{"-C"}, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := directoryFromArgs(tc.args)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func runBalances(t *testing.T, args ...string) string {
	t.Helper()
	argv := append([]string{"balances"}, args...)
	app := cmd.ParseOK[cmd.App[root]](t, argv...)
	return lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
}

func balancesHelp(t *testing.T, args ...string) string {
	t.Helper()
	app := cmd.ParseOK[cmd.App[root]](t, args...)
	return lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
}

func withArgs(t *testing.T, args ...string) {
	t.Helper()
	orig := os.Args
	t.Cleanup(func() { os.Args = orig })
	os.Args = args
}
