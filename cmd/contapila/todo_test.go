package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lewtec/lewkit/x/cmd"
	lewtest "github.com/lewtec/lewkit/x/test"
	lewreport "github.com/lewtec/lewkit/x/text/report"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTodoRuleAndMessage(t *testing.T) {
	item := engine.Todo{
		Date:      time.Date(2024, 5, 10, 0, 0, 0, 0, time.UTC),
		Flag:      "*",
		Payee:     "Cafe Moka",
		Narration: "Coffee",
		Tag:       true,
	}
	assert.Equal(t, "todo", todoRuleID(item))
	assert.Equal(t, `2024-05-10 * "Cafe Moka" "Coffee"`, todoMessage(item))

	item.Tag = false
	item.Payee = ""
	item.Narration = "out"
	item.Accounts = []string{"Expenses:TODO", "Income:TODO"}
	item.Date = time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, "Expenses:TODO,Income:TODO", todoRuleID(item))
	assert.Equal(t, `2020-03-01 * "out"`, todoMessage(item))

	assert.Equal(t, "todo,Income:TODO,Expenses:TODO", todoRuleID(engine.Todo{
		Tag:      true,
		Accounts: []string{"Income:TODO", "Expenses:TODO"},
	}))
}

func TestTodoPath(t *testing.T) {
	assert.Equal(t, "personal/surface.beancount", todoPath("/proj", "/proj/personal/surface.beancount"))
	assert.Equal(t, "/other/main.beancount", todoPath("/proj", "/other/main.beancount"))
	assert.Equal(t, "", todoPath("/proj", ""))
}

func TestTodoFindingSpan(t *testing.T) {
	directory := t.TempDir()
	body := "2020-02-01 * \"Lunch\" \"tagged\" #todo\n  Expenses:Food\n"
	path := filepath.Join(directory, "main.beancount")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	newline := strings.IndexByte(body, '\n')
	require.Greater(t, newline, 0)
	first := body[:newline]

	item := engine.Todo{
		Date:      time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC),
		File:      path,
		Line:      1,
		Flag:      "*",
		Payee:     "Lunch",
		Narration: "tagged",
		Tag:       true,
		StartByte: 0,
		EndByte:   len(first),
	}
	got, err := todoFinding(map[string][]byte{}, directory, item)
	require.NoError(t, err)
	assert.Equal(t, "todo", got.RuleID)
	assert.Equal(t, lewreport.LevelWarning, got.Level)
	assert.Equal(t, `2020-02-01 * "Lunch" "tagged"`, got.Message)
	assert.Equal(t, "main.beancount", got.File)
	assert.Equal(t, 1, got.Line)
	assert.Equal(t, 1, got.Column)
	assert.Equal(t, first, got.Snippet)
	assert.Equal(t, []byte(body), got.Source)

	item.EndByte = len(body)
	got, err = todoFinding(map[string][]byte{}, directory, item)
	require.NoError(t, err)
	assert.Equal(t, first+"…", got.Snippet)
	assert.Greater(t, got.EndLine, got.Line)

	item.StartByte = 0
	item.EndByte = 0
	got, err = todoFinding(map[string][]byte{}, directory, item)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Column)
	assert.Equal(t, first, got.Snippet)

	item.EndByte = len(body) + 8
	got, err = todoFinding(map[string][]byte{}, directory, item)
	require.NoError(t, err)
	assert.Equal(t, first, got.Snippet)

	item.File = filepath.Join(directory, "missing.beancount")
	_, err = todoFinding(map[string][]byte{}, directory, item)
	require.Error(t, err)
}

func TestTodoHelp(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "todo", "--help")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "--format")
	assert.Contains(t, out, "rustc")
	assert.Contains(t, out, "table")
	assert.Contains(t, out, "case-sensitive")
	assert.Contains(t, out, "exactly two components")
	assert.Contains(t, out, "Expenses:TODO:Tax")
	assert.Contains(t, out, "Expenses:Food:TODO")
	assert.Contains(t, out, "Either posting direction matches.")
	assert.Contains(t, out, "listed once")
}

func TestTodoFormatRejected(t *testing.T) {
	err := cmd.ParseErr[cmd.App[root]](t, "todo", "--format", "jsonl")
	require.Error(t, err)
}

func TestTodoExample(t *testing.T) {
	lewtest.DiscardSlog(t)
	directory := exampleDir(t)

	app := cmd.ParseOK[cmd.App[root]](t, "-C", directory, "todo", "personal")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, `warning[todo]: 2024-05-10 * "Cafe Moka" "Coffee"`)
	assert.Contains(t, out, "--> personal/surface.beancount:13:")
	assert.Contains(t, out, `2024-05-10 * "Cafe Moka" "Coffee" #todo`)
	assert.Contains(t, out, "Expenses:Prazeres:Cafe")
	assert.Contains(t, out, `warning[todo]: 2025-12-15 * "Padaria Central" "Bread"`)
	assert.Contains(t, out, "--> personal/controls.beancount:59:")
	assert.Contains(t, out, "\x1b[4m")
	assert.NotContains(t, out, "review FX lots")
	assert.NotContains(t, out, "== personal ==")
	assert.NotContains(t, out, "== acme ==")
	coffee := strings.Index(out, "Coffee")
	bread := strings.Index(out, "Bread")
	assert.Greater(t, coffee, 0)
	assert.Greater(t, bread, coffee)

	app = cmd.ParseOK[cmd.App[root]](t, "-C", directory, "todo", "--format", "table", "personal")
	out = lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	for _, column := range []string{"LOCATION", "LEVEL", "RULE", "MESSAGE", "FIX"} {
		assert.Contains(t, out, column)
	}
	assert.Contains(t, out, "personal/surface.beancount:13:")
	assert.Contains(t, out, "personal/controls.beancount:59:")
	assert.Contains(t, out, "warning")
	assert.Contains(t, out, "todo")
	assert.Contains(t, out, `2024-05-10 * "Cafe Moka" "Coffee"`)
	assert.NotContains(t, out, "warning[todo]")
	assert.NotContains(t, out, "\x1b[4m")
	assert.Less(t, strings.Index(out, "Coffee"), strings.Index(out, "Bread"))

	app = cmd.ParseOK[cmd.App[root]](t, "-C", directory, "todo", "personal", "--format", "text")
	out = lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, `personal/surface.beancount:13:1: warning: todo: 2024-05-10 * "Cafe Moka" "Coffee"`)
	assert.Contains(t, out, `personal/controls.beancount:59:1: warning: todo: 2025-12-15 * "Padaria Central" "Bread"`)

	app = cmd.ParseOK[cmd.App[root]](t, "-C", directory, "todo", "personal", "--format", "sarif")
	out = lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, `"version": "2.1.0"`)
	assert.Contains(t, out, `"name": "contapila"`)
	assert.Contains(t, out, "https://github.com/lewtec/contapila")
	assert.Contains(t, out, "personal/surface.beancount")
	assert.Contains(t, out, `"ruleId": "todo"`)
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
	assert.Contains(t, out, `warning[todo]: 2020-02-01 ! "Lunch" "tagged"`)
	assert.Contains(t, out, "--> personal/main.beancount:14:")
	assert.Contains(t, out, `warning[Expenses:TODO]: 2020-03-01 * "Mystery" "out"`)
	assert.Contains(t, out, "--> personal/main.beancount:18:")
	assert.Contains(t, out, `warning[Income:TODO]: 2020-04-01 * "Refund" "in"`)
	assert.Contains(t, out, "--> personal/main.beancount:22:")
	assert.Contains(t, out, `warning[todo,Expenses:TODO]: 2020-05-01 * "Both" "marks"`)
	assert.Contains(t, out, "--> personal/main.beancount:26:")
	assert.Contains(t, out, `warning[Type:TODO]: 2020-08-01 * "Literal" "type"`)
	assert.Contains(t, out, "--> personal/main.beancount:38:")
	assert.Contains(t, out, `warning[todo]: 2020-09-01 * "Office" "pens"`)
	assert.Contains(t, out, "--> company/main.beancount:4:")
	assert.NotContains(t, out, "Seed")
	assert.NotContains(t, out, "Deep")
	assert.NotContains(t, out, "Plain")
	assert.NotContains(t, out, "== personal ==")
	assert.NotContains(t, out, "== company ==")
	assert.Less(t, strings.Index(out, "tagged"), strings.Index(out, "Mystery"))
	assert.Less(t, strings.Index(out, "Mystery"), strings.Index(out, "Refund"))
	assert.Less(t, strings.Index(out, "Refund"), strings.Index(out, "marks"))
	assert.Less(t, strings.Index(out, "marks"), strings.Index(out, "Literal"))
	assert.Less(t, strings.Index(out, "Literal"), strings.Index(out, "Office"))

	app = cmd.ParseOK[cmd.App[root]](t, "-C", directory, "todo", "personal")
	out = lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "personal/main.beancount")
	assert.NotContains(t, out, "company/main.beancount")
	assert.NotContains(t, out, "Office")
}

func TestTodoEmpty(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "contapila.cue"), []byte("// empty\n"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(directory, "personal"), 0o755))
	body := strings.TrimPrefix(`
option "operating_currency" "BRL"
1970-01-01 open Assets:Cash BRL
1970-01-01 open Equity:Opening BRL
2020-01-01 * "Seed"
  Assets:Cash  1.00 BRL
  Equity:Opening
`, "\n")
	require.NoError(t, os.WriteFile(filepath.Join(directory, "personal", "main.beancount"), []byte(body), 0o644))

	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", directory, "todo")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Empty(t, out)

	app = cmd.ParseOK[cmd.App[root]](t, "-C", directory, "todo", "--format", "table")
	out = lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "LOCATION")
	assert.NotContains(t, out, "Seed")
}

func TestTodoUnknownLedger(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", exampleDir(t), "todo", "nope")
	require.ErrorIs(t, app.Run(t.Context()), engine.ErrUnknownLedger)
}
