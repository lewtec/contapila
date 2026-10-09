package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lucasew/contapila-go/internal/engine"
)

type todoCmd struct {
	Ledger *engine.LedgerArg
}

func (todoCmd) Description() string {
	return `List transactions tagged #todo or posting to Type:TODO

Type:TODO is an account with two components whose second component is TODO
(Expenses:TODO, Income:TODO, Equity:TODO). A transaction is listed once.
Each line is file:line:, oldest first.`
}

func (c *todoCmd) Run(ctx context.Context) error {
	return withLedgers(ctx, optionalName(c.Ledger), func(l *engine.Ledger) error {
		fmt.Printf("== %s ==\n", l.Name)
		root := ""
		if l.Project != nil {
			root = l.Project.Root
		}
		for _, item := range l.Todos() {
			fmt.Println(formatTodo(root, item))
		}
		return nil
	})
}

func formatTodo(root string, item engine.Todo) string {
	var parts []string
	if loc := todoLocation(root, item.File, item.Line); loc != "" {
		parts = append(parts, loc)
	}
	if !item.Date.IsZero() {
		parts = append(parts, item.Date.Format("2006-01-02"))
	}
	if item.Flag != "" {
		parts = append(parts, item.Flag)
	}
	parts = append(parts, formatPayeeNarration(item.Payee, item.Narration))
	if item.Tag {
		parts = append(parts, "#todo")
	}
	parts = append(parts, item.Accounts...)
	return strings.Join(parts, " ")
}

func todoLocation(root, file string, line int) string {
	path := todoPath(root, file)
	if path == "" {
		return ""
	}
	if line > 0 {
		return fmt.Sprintf("%s:%d:", path, line)
	}
	return path + ":"
}

func todoPath(root, file string) string {
	if file == "" {
		return ""
	}
	if root == "" {
		return filepath.ToSlash(file)
	}
	rel, err := filepath.Rel(root, file)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(file)
	}
	return filepath.ToSlash(rel)
}
