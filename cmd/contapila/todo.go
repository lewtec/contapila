package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lewtec/lewkit/x/cmd"
	lewreport "github.com/lewtec/lewkit/x/text/report"
	"github.com/lucasew/contapila-go/internal/engine"
)

type todoCmd struct {
	Format cmd.EnumArg[lewreport.Format] `long:"format" default:"rustc" help:"diagnostic format"`
	Ledger *engine.LedgerArg
}

func (todoCmd) Description() string {
	return `List transactions tagged #todo or posting to Type:TODO

Type:TODO is an account with two components whose second component is TODO
(Expenses:TODO, Income:TODO, Equity:TODO). A transaction is listed once,
oldest first. The default format is rustc.`
}

func (c *todoCmd) Run(ctx context.Context) error {
	var root string
	var items []engine.Todo
	err := withLedgers(ctx, optionalName(c.Ledger), func(l *engine.Ledger) error {
		if root == "" && l.Project != nil {
			root = l.Project.Root
		}
		items = append(items, l.Todos()...)
		return nil
	})
	if err != nil {
		return err
	}
	sort.SliceStable(items, func(i, j int) bool {
		return earlierTodo(items[i], items[j])
	})
	findings, err := todoFindings(root, items)
	if err != nil {
		return err
	}
	return c.Format.Value().Render(os.Stdout, root, lewreport.Tool{
		Name:           "contapila",
		InformationURI: "https://github.com/lewtec/contapila",
	}, findings, nil)
}

func earlierTodo(a, b engine.Todo) bool {
	if !a.Date.Equal(b.Date) {
		return a.Date.Before(b.Date)
	}
	if a.File != b.File {
		return a.File < b.File
	}
	return a.Line < b.Line
}

// todoRuleID is the tag todo, then Type:TODO accounts, comma-separated.
func todoRuleID(item engine.Todo) string {
	var marks []string
	if item.Tag {
		marks = append(marks, "todo")
	}
	marks = append(marks, item.Accounts...)
	return strings.Join(marks, ",")
}

func todoMessage(item engine.Todo) string {
	var parts []string
	if !item.Date.IsZero() {
		parts = append(parts, item.Date.Format("2006-01-02"))
	}
	if item.Flag != "" {
		parts = append(parts, item.Flag)
	}
	parts = append(parts, formatPayeeNarration(item.Payee, item.Narration))
	return strings.Join(parts, " ")
}

func todoFindings(root string, items []engine.Todo) ([]lewreport.Finding, error) {
	if len(items) == 0 {
		return nil, nil
	}
	cache := map[string][]byte{}
	out := make([]lewreport.Finding, 0, len(items))
	for _, item := range items {
		finding, err := todoFinding(cache, root, item)
		if err != nil {
			return nil, err
		}
		out = append(out, finding)
	}
	return out, nil
}

func todoFinding(cache map[string][]byte, root string, item engine.Todo) (lewreport.Finding, error) {
	line := item.Line
	if line < 1 {
		line = 1
	}
	finding := lewreport.Finding{
		RuleID:  todoRuleID(item),
		Level:   lewreport.LevelWarning,
		Message: todoMessage(item),
		File:    todoPath(root, item.File),
		Line:    line,
		Column:  1,
		EndLine: line,
	}
	source, err := todoSource(cache, item.File)
	if err != nil {
		return lewreport.Finding{}, err
	}
	if source == nil {
		return finding, nil
	}
	finding.Source = source
	if span, ok := todoSpan(item, len(source)); ok {
		spanLine, column, endLine, endColumn, snippet, spanErr := lewreport.SpanLoc(source, span)
		if spanErr == nil {
			finding.Line = spanLine
			finding.Column = column
			finding.EndLine = endLine
			finding.EndCol = endColumn
			finding.Snippet = snippet
			return finding, nil
		}
	}
	finding.Snippet = todoLine(source, finding.Line)
	return finding, nil
}

func todoSpan(item engine.Todo, size int) (lewreport.Span, bool) {
	start, end := item.StartByte, item.EndByte
	if start < 0 || end < 0 || end <= start || end > size {
		return lewreport.Span{}, false
	}
	if uint64(start) > math.MaxUint32 || uint64(end) > math.MaxUint32 {
		return lewreport.Span{}, false
	}
	return lewreport.Span{StartByte: uint32(start), EndByte: uint32(end)}, true
}

func todoSource(cache map[string][]byte, file string) ([]byte, error) {
	if file == "" {
		return nil, nil
	}
	if source, ok := cache[file]; ok {
		return source, nil
	}
	source, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	cache[file] = source
	return source, nil
}

func todoLine(source []byte, line int) string {
	if line < 1 {
		return ""
	}
	lines := strings.Split(string(source), "\n")
	if line > len(lines) {
		return ""
	}
	return lines[line-1]
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
