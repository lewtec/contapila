package main

import (
	"testing"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegexpArgMatch(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		text    string
		want    bool
	}{
		{name: "substring", pattern: "Alfa", text: "Assets:BR:Alfa:ContaCorrente", want: true},
		{name: "substring miss", pattern: "Alfa", text: "Assets:Cash:Carteira", want: false},
		{name: "colon literal", pattern: "A:B", text: "xA:By", want: true},
		{name: "colon not any", pattern: "A:B", text: "AxB", want: false},
		{name: "dot any", pattern: "A.B", text: "A:B", want: true},
		{name: "anchor start", pattern: "^Assets", text: "Assets:Cash", want: true},
		{name: "anchor start miss", pattern: "^Cash", text: "Assets:Cash", want: false},
		{name: "anchor end", pattern: "Cash$", text: "Assets:Cash", want: true},
		{name: "anchor end miss", pattern: "Cash$", text: "Assets:Cash:Wallet", want: false},
		{name: "exact", pattern: "^Assets:Cash$", text: "Assets:Cash", want: true},
		{name: "exact longer", pattern: "^Assets:Cash$", text: "Assets:Cash:Wallet", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var arg RegexpArg
			require.NoError(t, arg.Parse(tc.pattern))
			assert.Equal(t, tc.want, arg.MatchString(tc.text))
			assert.Equal(t, tc.pattern, arg.Value().String())
		})
	}
}

func TestRegexpArgRejects(t *testing.T) {
	var empty RegexpArg
	err := empty.Parse("")
	require.ErrorIs(t, err, cmd.ErrInvalidArgument)
	assert.ErrorIs(t, err, errEmptyPattern)
	assert.Nil(t, empty.Value())
	assert.False(t, empty.MatchString("anything"))

	var bad RegexpArg
	err = bad.Parse("[")
	require.ErrorIs(t, err, cmd.ErrInvalidArgument)
	assert.Nil(t, bad.Value())

	var zero RegexpArg
	assert.Nil(t, zero.Value())
	assert.False(t, zero.MatchString("Assets"))
}

func TestRegexpArgFlag(t *testing.T) {
	type patternFlag struct {
		Pattern RegexpArg `long:"pattern"`
	}
	got := cmd.ParseOK[patternFlag](t, "--pattern", "Alfa")
	assert.True(t, got.Pattern.MatchString("Assets:BR:Alfa:ContaCorrente"))
	assert.False(t, got.Pattern.MatchString("Assets:Cash:Carteira"))

	err := cmd.ParseErr[patternFlag](t, "--pattern", "")
	require.ErrorIs(t, err, cmd.ErrInvalidArgument)

	err = cmd.ParseErr[patternFlag](t, "--pattern", "[")
	require.ErrorIs(t, err, cmd.ErrInvalidArgument)

	type repeatFlag struct {
		Account []RegexpArg `long:"account"`
	}
	rep := cmd.ParseOK[repeatFlag](t, "--account", "Cash", "--account", "Food$")
	filters := cmd.Values(rep.Account)
	require.Len(t, filters, 2)
	assert.True(t, filters[0].MatchString("Assets:Cash"))
	assert.False(t, filters[0].MatchString("Assets:Bank"))
	assert.True(t, filters[1].MatchString("Expenses:Food"))
	assert.False(t, filters[1].MatchString("Expenses:Food:Snack"))
}
