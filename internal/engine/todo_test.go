package engine

import (
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTodosNilLedger(t *testing.T) {
	var ledger *Ledger
	assert.Nil(t, ledger.Todos())
}

func TestTodos(t *testing.T) {
	day := func(s string) time.Time {
		t.Helper()
		parsed, err := time.Parse("2006-01-02", s)
		require.NoError(t, err)
		return parsed
	}
	txn := func(date, file string, line int, flag, payee, narration string, tags []string, accounts ...string) ast.Transaction {
		var postings []ast.Posting
		for _, account := range accounts {
			postings = append(postings, ast.Posting{Account: account})
		}
		return ast.Transaction{
			Meta:      ast.Meta{Date: day(date), File: file, Line: line, StartByte: line, EndByte: line + 3},
			Flag:      flag,
			Payee:     payee,
			Narration: narration,
			Tags:      tags,
			Postings:  postings,
		}
	}
	ledger := &Ledger{dirs: []ast.Directive{
		txn("2020-05-01", "b.beancount", 5, "*", "Cafe", "tagged", []string{"todo"}, "Assets:Cash", "Expenses:Food"),
		txn("2020-01-02", "a.beancount", 9, "!", "Both", "marks", []string{"todo"}, "Income:TODO", "Expenses:TODO", "Expenses:TODO"),
		txn("2020-01-02", "a.beancount", 2, "*", "Mystery", "out", nil, "Assets:Cash", "Expenses:TODO"),
		txn("2020-01-03", "a.beancount", 3, "*", "Skip", "deep", []string{"TODO", "todos"}, "Expenses:TODO:Tax", "Expenses:Food:TODO"),
		txn("2020-01-04", "a.beancount", 4, "*", "Skip", "plain", nil, "Assets:Cash", "Expenses:Food"),
		txn("2020-01-04", "a.beancount", 8, "*", "Literal", "type", nil, "Type:TODO"),
		ast.Note{Meta: ast.Meta{Date: day("2020-01-01"), File: "a.beancount", Line: 1}, Comment: "#todo"},
	}}

	got := ledger.Todos()
	want := []Todo{
		{Date: day("2020-01-02"), File: "a.beancount", Line: 2, StartByte: 2, EndByte: 5, Flag: "*", Payee: "Mystery", Narration: "out", Accounts: []string{"Expenses:TODO"}},
		{Date: day("2020-01-02"), File: "a.beancount", Line: 9, StartByte: 9, EndByte: 12, Flag: "!", Payee: "Both", Narration: "marks", Tag: true, Accounts: []string{"Income:TODO", "Expenses:TODO"}},
		{Date: day("2020-01-04"), File: "a.beancount", Line: 8, StartByte: 8, EndByte: 11, Flag: "*", Payee: "Literal", Narration: "type", Accounts: []string{"Type:TODO"}},
		{Date: day("2020-05-01"), File: "b.beancount", Line: 5, StartByte: 5, EndByte: 8, Flag: "*", Payee: "Cafe", Narration: "tagged", Tag: true},
	}
	require.Len(t, got, len(want))
	for i := range want {
		assert.Equal(t, want[i], got[i])
	}
}

func TestTodosEmpty(t *testing.T) {
	ledger := &Ledger{dirs: []ast.Directive{
		ast.Transaction{
			Meta:     ast.Meta{Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), File: "a.beancount", Line: 1},
			Flag:     "*",
			Postings: []ast.Posting{{Account: "Expenses:Food"}},
		},
	}}
	assert.Nil(t, ledger.Todos())
}
