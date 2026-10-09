package engine

import (
	"sort"
	"strings"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
)

// Todo is one Transaction the todo command lists.
// Tag is set when the Transaction carries the tag todo (#todo).
// Accounts are Postings to Type:TODO, in source order, without duplicates.
type Todo struct {
	Date      time.Time
	File      string
	Line      int
	Flag      string
	Payee     string
	Narration string
	Tag       bool
	Accounts  []string
}

// Todos lists Transactions tagged #todo or posting to Type:TODO.
// Type:TODO is an Account of exactly two components whose second component
// is TODO (Expenses:TODO, Income:TODO, Equity:TODO). The literal Account
// Type:TODO matches the same rule. Order is date, then file, then line.
func (l *Ledger) Todos() []Todo {
	if l == nil {
		return nil
	}
	var out []Todo
	for _, d := range l.dirs {
		txn, ok := d.(ast.Transaction)
		if !ok {
			continue
		}
		item, ok := todoItem(txn)
		if !ok {
			continue
		}
		out = append(out, item)
	}
	// Oldest first, so the backlog starts at the top.
	sort.SliceStable(out, func(i, j int) bool {
		return todoBefore(out[i], out[j])
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func todoItem(txn ast.Transaction) (Todo, bool) {
	item := Todo{
		Date:      txn.Date,
		File:      txn.File,
		Line:      txn.Line,
		Flag:      txn.Flag,
		Payee:     txn.Payee,
		Narration: txn.Narration,
		Tag:       hasTodoTag(txn.Tags),
	}
	seen := map[string]bool{}
	for _, posting := range txn.Postings {
		if !typeTODO(posting.Account) || seen[posting.Account] {
			continue
		}
		seen[posting.Account] = true
		item.Accounts = append(item.Accounts, posting.Account)
	}
	if !item.Tag && len(item.Accounts) == 0 {
		return Todo{}, false
	}
	return item, true
}

func hasTodoTag(tags []string) bool {
	for _, tag := range tags {
		if tag == "todo" {
			return true
		}
	}
	return false
}

// typeTODO reports whether account is Type:TODO.
// Expenses:TODO matches. Expenses:TODO:Tax and Expenses:Food:TODO do not.
func typeTODO(account string) bool {
	accountType, name, ok := strings.Cut(account, ":")
	return ok && accountType != "" && name == "TODO"
}

func todoBefore(a, b Todo) bool {
	if !a.Date.Equal(b.Date) {
		return a.Date.Before(b.Date)
	}
	if a.File != b.File {
		return a.File < b.File
	}
	return a.Line < b.Line
}
