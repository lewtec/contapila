package lsp

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func TestLSP_DefinitionCompletionHover(t *testing.T) {
	root := t.TempDir()
	// minimal contapila project
	if err := os.WriteFile(filepath.Join(root, "contapila.cue"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledgerDir := filepath.Join(root, "personal")
	if err := os.MkdirAll(ledgerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(ledgerDir, "main.beancount")
	src := strings.Join([]string{
		`2020-01-01 open Assets:Cash BRL`,
		`2020-01-01 open Equity:OpeningBalances BRL`,
		`2020-01-01 commodity BRL`,
		``,
		`2020-01-02 * "seed"`,
		`  Assets:Cash  10 BRL`,
		`  Equity:OpeningBalances  -10 BRL`,
		``,
	}, "\n")
	if err := os.WriteFile(mainPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	srv, connA, _ := RunWith(ctx, jsonrpc2.NewHeaderStream(a))
	defer connA.Close()
	_ = srv

	_, connB, client := protocol.NewClient(ctx, protocol.UnimplementedClient{}, jsonrpc2.NewHeaderStream(b))
	defer connB.Close()

	// initialize
	_, err := client.Initialize(ctx, &protocol.InitializeParams{})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := client.Initialized(ctx, &protocol.InitializedParams{}); err != nil {
		t.Fatalf("initialized: %v", err)
	}

	docURI := uri.File(mainPath)
	if err := client.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        docURI,
			LanguageID: "beancount",
			Version:    1,
			Text:       src,
		},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	// wait for snapshot rebuild
	deadline := time.Now().Add(5 * time.Second)
	for {
		snap := srv.session.snapshot()
		if snap != nil && len(snap.Accounts["personal"]) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for snapshot accounts")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Definition on Assets:Cash in posting line
	// Find position of second Assets:Cash (posting)
	idx := strings.LastIndex(src, "Assets:Cash")
	if idx < 0 {
		t.Fatal("token not found")
	}
	pos := offsetToPos(src, idx+1)
	def, err := client.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("definition: %v", err)
	}
	loc, ok := def.(*protocol.Location)
	if !ok || loc == nil {
		t.Fatalf("definition result %#v", def)
	}
	if !strings.Contains(string(loc.URI), "main.beancount") {
		t.Fatalf("def uri %s", loc.URI)
	}

	// Completion on empty posting indent after last line — use position after "  " of a new posting context
	// Put cursor after "  Ass" partial on a fabricated change
	partial := src + "  Ass"
	if err := client.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: docURI},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			&protocol.TextDocumentContentChangeWholeDocument{Text: partial},
		},
	}); err != nil {
		t.Fatalf("didChange: %v", err)
	}
	// snapshot may still be last-good with accounts
	off := len(partial)
	cpos := offsetToPos(partial, off)
	comp, err := client.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     cpos,
		},
	})
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	items, ok := comp.(protocol.CompletionItemSlice)
	if !ok {
		t.Fatalf("completion type %T", comp)
	}
	found := false
	for _, it := range items {
		if it.Label == "Assets:Cash" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Assets:Cash in completion, got %v", labels(items))
	}

	// Hover on BRL commodity in open line
	idx = strings.Index(src, " BRL")
	if idx < 0 {
		t.Fatal("BRL not found")
	}
	// point at B of BRL after open Assets:Cash
	hpos := offsetToPos(src, idx+1)
	// restore full text for hover on original
	_ = client.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: docURI},
			Version:                3,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			&protocol.TextDocumentContentChangeWholeDocument{Text: src},
		},
	})
	// wait a bit so overlay has src; hover uses overlay + snap
	time.Sleep(100 * time.Millisecond)
	hover, err := client.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     hpos,
		},
	})
	if err != nil {
		t.Fatalf("hover: %v", err)
	}
	if hover == nil {
		t.Fatal("nil hover")
	}
	body := hoverText(hover)
	if !strings.Contains(body, "commodity") && !strings.Contains(body, "BRL") && !strings.Contains(body, "precision") {
		// open line "Assets:Cash BRL" — token may be BRL
		if !strings.Contains(body, "BRL") {
			t.Fatalf("hover body %q", body)
		}
	}
}

func labels(items protocol.CompletionItemSlice) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Label
	}
	return out
}

func hoverText(h *protocol.Hover) string {
	switch c := h.Contents.(type) {
	case protocol.String:
		return string(c)
	case *protocol.MarkupContent:
		return c.Value
	default:
		return ""
	}
}

func offsetToPos(text string, byteOff int) protocol.Position {
	if byteOff > len(text) {
		byteOff = len(text)
	}
	line, col := 0, 0
	for i := 0; i < byteOff; i++ {
		if text[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return protocol.Position{Line: uint32(line), Character: uint32(col)}
}

func TestTokenAndCompletionKind(t *testing.T) {
	if completionKind("  Assets:C") != "account" {
		t.Fatalf("posting account")
	}
	if completionKind("2020-01-01 open Ass") != "account" {
		t.Fatalf("open account")
	}
	if completionKind("2020-01-01 commodity B") != "commodity" {
		t.Fatalf("commodity")
	}
	if completionKind("  Assets:Cash  10 B") != "commodity" {
		t.Fatalf("amount commodity")
	}
	if completionKind("") != "date" {
		t.Fatalf("empty line date")
	}
	if completionKind("2024") != "date" {
		t.Fatalf("year prefix date")
	}
	if completionKind("2024-01-1") != "date" {
		t.Fatalf("partial day date")
	}
	if completionKind("2024-01-15") != "" {
		t.Fatalf("complete date alone is not date-complete")
	}
}

func TestSuggestDates(t *testing.T) {
	now := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	doc := "2024-03-10 open Assets:Cash\n2024-03-12 * \"x\"\n"
	got := suggestDates("", doc, now)
	if len(got) < 3 {
		t.Fatalf("expected several dates, got %v", got)
	}
	if got[0].Date != "2024-03-15" || got[0].Detail != "today" {
		t.Fatalf("first want today 2024-03-15, got %#v", got[0])
	}
	if got[1].Date != "2024-03-14" || got[1].Detail != "yesterday" {
		t.Fatalf("second want yesterday, got %#v", got[1])
	}
	if got[2].Date != "2024-03-01" || got[2].Detail != "start of month" {
		t.Fatalf("third want start of month 2024-03-01, got %#v", got[2])
	}
	// prefix filter
	pref := suggestDates("2024-03-1", doc, now)
	for _, d := range pref {
		if !strings.HasPrefix(d.Date, "2024-03-1") {
			t.Fatalf("prefix filter failed: %s", d.Date)
		}
	}
	// file date included
	found := false
	for _, d := range got {
		if d.Date == "2024-03-10" && d.Detail == "in file" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected in-file date 2024-03-10 in %v", got)
	}
}

func TestLSP_DateCompletion(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "contapila.cue"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledgerDir := filepath.Join(root, "personal")
	if err := os.MkdirAll(ledgerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(ledgerDir, "main.beancount")
	src := "2020-01-01 open Assets:Cash BRL\n\n"
	if err := os.WriteFile(mainPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	srv, connA, _ := RunWith(ctx, jsonrpc2.NewHeaderStream(a))
	defer connA.Close()
	_ = srv
	_, connB, client := protocol.NewClient(ctx, protocol.UnimplementedClient{}, jsonrpc2.NewHeaderStream(b))
	defer connB.Close()

	if _, err := client.Initialize(ctx, &protocol.InitializeParams{}); err != nil {
		t.Fatal(err)
	}
	_ = client.Initialized(ctx, &protocol.InitializedParams{})

	docURI := uri.File(mainPath)
	if err := client.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: "beancount", Version: 1, Text: src,
		},
	}); err != nil {
		t.Fatal(err)
	}
	// didOpen is a notification — wait until the server has the overlay
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := srv.session.docText(mainPath); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for didOpen overlay")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// cursor on empty third line
	pos := offsetToPos(src, len(src))
	comp, err := client.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	items, ok := comp.(protocol.CompletionItemSlice)
	if !ok || len(items) == 0 {
		t.Fatalf("want date items, got %T %v", comp, comp)
	}
	// today should be first-ish
	today := time.Now().Format("2006-01-02")
	foundToday := false
	foundFile := false
	for _, it := range items {
		if it.Label == today {
			foundToday = true
		}
		if it.Label == "2020-01-01" {
			foundFile = true
		}
	}
	if !foundToday {
		t.Fatalf("missing today %s in %v", today, labels(items))
	}
	if !foundFile {
		t.Fatalf("missing in-file 2020-01-01 in %v", labels(items))
	}
}
