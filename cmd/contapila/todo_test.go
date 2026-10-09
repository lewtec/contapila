package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lewtec/lewkit/x/cmd"
	lewtest "github.com/lewtec/lewkit/x/test"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatTodo(t *testing.T) {
	got := formatTodo("/proj", engine.Todo{
		Date:      time.Date(2024, 5, 10, 0, 0, 0, 0, time.UTC),
		File:      "/proj/personal/surface.beancount",
		Line:      13,
		Flag:      "*",
		Payee:     "Cafe Moka",
		Narration: "Coffee",
		Tag:       true,
	})
	assert.Equal(t, `personal/surface.beancount:13: 2024-05-10 * "Cafe Moka" "Coffee" #todo`, got)

	got = formatTodo("/proj", engine.Todo{
		Date:      time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC),
		File:      "/proj/personal/main.beancount",
		Line:      8,
		Flag:      "*",
		Narration: "out",
		Accounts:  []string{"Expenses:TODO", "Income:TODO"},
	})
	assert.Equal(t, `personal/main.beancount:8: 2020-03-01 * "out" Expenses:TODO Income:TODO`, got)

	got = formatTodo("/proj", engine.Todo{
		Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		File: "/other/main.beancount",
		Line: 4,
		Flag: "!",
		Tag:  true,
	})
	assert.Equal(t, `/other/main.beancount:4: 2020-01-01 ! "" #todo`, got)
}

func TestTodoExample(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", exampleDir(t), "todo", "personal")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "== personal ==")
	assert.NotContains(t, out, "== acme ==")
	assert.Contains(t, out, `personal/surface.beancount:13: 2024-05-10 * "Cafe Moka" "Coffee" #todo`)
	assert.Contains(t, out, `personal/controls.beancount:59: 2025-12-15 * "Padaria Central" "Bread" #todo`)
	assert.NotContains(t, out, "review FX lots")
	coffee := strings.Index(out, "Coffee")
	bread := strings.Index(out, "Bread")
	assert.Greater(t, coffee, 0)
	assert.Greater(t, bread, coffee)
}

func TestTodoTypeAccount(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "contapila.cue"), []byte("// empty\n"), 0o644))
	for _, name := range []string{"personal", "company"} {
		require.NoError(t, os.Mkdir(filepath.Join(directory, name), 0o755))
	}
	personal := strings.TrimPrefix(`
option "operating_currency" "BRL"
1970-01-01 open Assets:Cash BRL
1970-01-01 open Expenses:Food BRL
1970-01-01 open Expenses:TODO BRL
1970-01-01 open Expenses:TODO:Tax BRL
1970-01-01 open Income:TODO BRL
1970-01-01 open Equity:Opening BRL
1970-01-01 open Type:TODO BRL

2020-01-01 * "Seed"
  Assets:Cash  100.00 BRL
  Equity:Opening

2020-02-01 ! "Lunch" "tagged" #todo
  Assets:Cash   -10.00 BRL
  Expenses:Food

2020-03-01 * "Mystery" "out"
  Assets:Cash    -5.00 BRL
  Expenses:TODO

2020-04-01 * "Refund" "in"
  Assets:Cash   3.00 BRL
  Income:TODO

2020-05-01 * "Both" "marks" #todo
  Assets:Cash    -1.00 BRL
  Expenses:TODO

2020-06-01 * "Deep" "nested"
  Assets:Cash    -2.00 BRL
  Expenses:TODO:Tax

2020-07-01 * "Plain" "food"
  Assets:Cash    -4.00 BRL
  Expenses:Food

2020-08-01 * "Literal" "type"
  Assets:Cash    -6.00 BRL
  Type:TODO
`, "\n")
	company := strings.TrimPrefix(`
option "operating_currency" "BRL"
2020-01-01 open Assets:Cash BRL
2020-01-01 open Expenses:Food BRL
2020-09-01 * "Office" "pens" #todo
  Assets:Cash  -2.00 BRL
  Expenses:Food
`, "\n")
	require.NoError(t, os.WriteFile(filepath.Join(directory, "personal", "main.beancount"), []byte(personal), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "company", "main.beancount"), []byte(company), 0o644))

	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", directory, "todo")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "== personal ==")
	assert.Contains(t, out, "== company ==")
	assert.Contains(t, out, `personal/main.beancount:14: 2020-02-01 ! "Lunch" "tagged" #todo`)
	assert.Contains(t, out, `personal/main.beancount:18: 2020-03-01 * "Mystery" "out" Expenses:TODO`)
	assert.Contains(t, out, `personal/main.beancount:22: 2020-04-01 * "Refund" "in" Income:TODO`)
	assert.Contains(t, out, `personal/main.beancount:26: 2020-05-01 * "Both" "marks" #todo Expenses:TODO`)
	assert.Contains(t, out, `personal/main.beancount:38: 2020-08-01 * "Literal" "type" Type:TODO`)
	assert.Contains(t, out, `company/main.beancount:4: 2020-09-01 * "Office" "pens" #todo`)
	assert.NotContains(t, out, "Seed")
	assert.NotContains(t, out, "Deep")
	assert.NotContains(t, out, "Plain")
	assert.Less(t, strings.Index(out, "tagged"), strings.Index(out, "Mystery"))
	assert.Less(t, strings.Index(out, "Mystery"), strings.Index(out, "Refund"))
	assert.Less(t, strings.Index(out, "Refund"), strings.Index(out, "marks"))
	assert.Less(t, strings.Index(out, "marks"), strings.Index(out, "Literal"))

	app = cmd.ParseOK[cmd.App[root]](t, "-C", directory, "todo", "personal")
	out = lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "== personal ==")
	assert.NotContains(t, out, "== company ==")
	assert.NotContains(t, out, "Office")
}

func TestTodoUnknownLedger(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", exampleDir(t), "todo", "nope")
	require.ErrorIs(t, app.Run(t.Context()), engine.ErrUnknownLedger)
}
