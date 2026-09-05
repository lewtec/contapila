package lsp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/pkg/version"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// Server is the contapila language server.
type Server struct {
	protocol.UnimplementedServer
	session *Session

	mu     sync.Mutex
	client protocol.Client
}

// New creates a server.
func New() *Server {
	return &Server{session: newSession()}
}

// RunStdio serves LSP over stdin/stdout.
func RunStdio(ctx context.Context) error {
	return Run(ctx, stdio{})
}

type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdio) Close() error                { return nil }

// Run serves over any ReadWriteCloser (tests use net.Pipe).
func Run(ctx context.Context, conn io.ReadWriteCloser) error {
	s := New()
	ctx, jconn, client := protocol.NewServer(ctx, s, jsonrpc2.NewHeaderStream(conn))
	s.mu.Lock()
	s.client = client
	s.session.setClient(client)
	s.mu.Unlock()
	// Block until connection closes.
	select {
	case <-ctx.Done():
		_ = jconn.Close()
		return ctx.Err()
	case <-jconn.Done():
		return jconn.Err()
	}
}

// RunWith returns server + client dispatcher for in-process tests.
func RunWith(ctx context.Context, stream jsonrpc2.Stream) (*Server, jsonrpc2.Conn, protocol.Client) {
	s := New()
	ctx2, jconn, client := protocol.NewServer(ctx, s, stream)
	_ = ctx2
	s.mu.Lock()
	s.client = client
	s.session.setClient(client)
	s.mu.Unlock()
	return s, jconn, client
}

func (s *Server) Initialize(_ context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	// root hint optional; first document still wins per SPEC
	if params.RootURI != nil {
		s.session.ensureRoot(uriToPath(*params.RootURI))
	}
	syncKind := protocol.TextDocumentSyncKindFull
	trueVal := protocol.Boolean(true)
	ver := version.GetBuildID()
	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync:   syncKind,
			CompletionProvider: &protocol.CompletionOptions{},
			HoverProvider:      trueVal,
			DefinitionProvider: trueVal,
		},
		ServerInfo: protocol.ServerInfo{
			Name:    "contapila",
			Version: protocol.NewOptional(ver),
		},
	}, nil
}

func (s *Server) Initialized(context.Context, *protocol.InitializedParams) error {
	return nil
}

func (s *Server) Shutdown(context.Context) error { return nil }

func (s *Server) Exit(context.Context) error { return nil }

func (s *Server) DidOpen(_ context.Context, params *protocol.DidOpenTextDocumentParams) error {
	path := uriToPath(params.TextDocument.URI)
	text := params.TextDocument.Text
	s.session.ensureRoot(path)
	s.session.overlay.Set(path, text)
	s.session.mu.Lock()
	s.session.openDocs[path] = params.TextDocument.Version
	s.session.mu.Unlock()
	s.session.parseBuffer(path, text)
	s.session.markDirty()
	return nil
}

func (s *Server) DidChange(_ context.Context, params *protocol.DidChangeTextDocumentParams) error {
	path := uriToPath(params.TextDocument.URI)
	if len(params.ContentChanges) == 0 {
		return nil
	}
	// full sync: last whole-document change text
	ch := params.ContentChanges[len(params.ContentChanges)-1]
	var text string
	switch c := ch.(type) {
	case *protocol.TextDocumentContentChangeWholeDocument:
		text = c.Text
	case *protocol.TextDocumentContentChangePartial:
		// full sync only; ignore partial for dogfood
		return nil
	default:
		return nil
	}
	s.session.overlay.Set(path, text)
	s.session.mu.Lock()
	s.session.openDocs[path] = params.TextDocument.Version
	s.session.mu.Unlock()
	s.session.parseBuffer(path, text)
	s.session.markDirty()
	return nil
}

func (s *Server) DidSave(_ context.Context, params *protocol.DidSaveTextDocumentParams) error {
	path := uriToPath(params.TextDocument.URI)
	if params.Text != nil {
		s.session.overlay.Set(path, *params.Text)
		s.session.parseBuffer(path, *params.Text)
	}
	// force rebuild now
	s.session.mu.Lock()
	s.session.dirty = true
	s.session.mu.Unlock()
	s.session.scheduleRebuild()
	return nil
}

func (s *Server) DidClose(_ context.Context, params *protocol.DidCloseTextDocumentParams) error {
	path := uriToPath(params.TextDocument.URI)
	s.session.overlay.Delete(path)
	s.session.mu.Lock()
	delete(s.session.openDocs, path)
	delete(s.session.parseDiags, path)
	s.session.mu.Unlock()
	s.session.markDirty()
	return nil
}

func (s *Server) Completion(_ context.Context, params *protocol.CompletionParams) (protocol.CompletionResult, error) {
	path := uriToPath(params.TextDocument.URI)
	text, ok := s.session.docText(path)
	if !ok {
		return protocol.CompletionItemSlice{}, nil
	}
	off := byteOffset(text, params.Position)
	prefix := linePrefixAt(text, off)
	kind := completionKind(prefix)
	tok, tokStart, _ := tokenAt(text, off)

	var items protocol.CompletionItemSlice
	switch kind {
	case "date":
		// Date tokens are digits/dashes; tokenAt already expands those runes.
		for _, d := range suggestDates(tok, text, time.Time{}) {
			te := &protocol.TextEdit{
				Range:   rangeFromBytes(text, tokStart, off, nil),
				NewText: d.Date,
			}
			items = append(items, protocol.CompletionItem{
				Label:    d.Date,
				Kind:     protocol.CompletionItemKindValue,
				Detail:   protocol.NewOptional(d.Detail),
				SortText: protocol.NewOptional(d.Sort),
				TextEdit: te,
			})
		}
		return items, nil
	case "account", "commodity":
		// need project snapshot
	default:
		return protocol.CompletionItemSlice{}, nil
	}

	snap := s.session.snapshot()
	if snap == nil {
		return protocol.CompletionItemSlice{}, nil
	}

	switch kind {
	case "account":
		ledger := s.session.resolveLedger(path)
		accs := snap.Accounts[ledger]
		names := make([]string, 0, len(accs))
		for n := range accs {
			if tok == "" || strings.HasPrefix(n, tok) || strings.Contains(strings.ToLower(n), strings.ToLower(tok)) {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		k := protocol.CompletionItemKindVariable
		for _, n := range names {
			te := &protocol.TextEdit{
				Range:   rangeFromBytes(text, tokStart, off, nil),
				NewText: n,
			}
			items = append(items, protocol.CompletionItem{
				Label:    n,
				Kind:     k,
				TextEdit: te,
			})
		}
	case "commodity":
		names := make([]string, 0, len(snap.Commodities)+len(snap.Policies))
		seen := map[string]bool{}
		for n := range snap.Commodities {
			seen[n] = true
			names = append(names, n)
		}
		for n := range snap.Policies {
			if !seen[n] {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		k := protocol.CompletionItemKindUnit
		for _, n := range names {
			if tok != "" && !strings.HasPrefix(n, tok) && !strings.Contains(strings.ToLower(n), strings.ToLower(tok)) {
				continue
			}
			te := &protocol.TextEdit{
				Range:   rangeFromBytes(text, tokStart, off, nil),
				NewText: n,
			}
			items = append(items, protocol.CompletionItem{
				Label:    n,
				Kind:     k,
				TextEdit: te,
			})
		}
	}
	return items, nil
}

func (s *Server) Definition(_ context.Context, params *protocol.DefinitionParams) (protocol.DefinitionResult, error) {
	path := uriToPath(params.TextDocument.URI)
	text, ok := s.session.docText(path)
	if !ok {
		return nil, nil
	}
	off := byteOffset(text, params.Position)
	tok, _, _ := tokenAt(text, off)
	if tok == "" || !strings.Contains(tok, ":") {
		// still try account-like without colon (unlikely)
		if tok == "" {
			return nil, nil
		}
	}
	snap := s.session.snapshot()
	if snap == nil {
		return nil, nil
	}
	ledger := s.session.resolveLedger(path)
	accs := snap.Accounts[ledger]
	info, ok := accs[tok]
	if !ok {
		return nil, nil
	}
	if info.File == "" {
		return nil, nil
	}
	// load text for range if possible
	defText, err := readMaybe(s, info.File)
	r := protocol.Range{
		Start: protocol.Position{Line: uint32(max(0, info.Line-1)), Character: 0},
		End:   protocol.Position{Line: uint32(max(0, info.Line-1)), Character: 1},
	}
	if err == nil && (info.StartByte > 0 || info.EndByte > info.StartByte) {
		r = rangeFromBytes(defText, info.StartByte, info.EndByte, nil)
	}
	loc := &protocol.Location{
		URI:   pathToURI(info.File),
		Range: r,
	}
	return loc, nil
}

func readMaybe(s *Server, path string) (string, error) {
	if t, ok := s.session.docText(path); ok {
		return t, nil
	}
	b, err := s.session.overlay.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *Server) Hover(_ context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	path := uriToPath(params.TextDocument.URI)
	text, ok := s.session.docText(path)
	if !ok {
		return nil, nil
	}
	off := byteOffset(text, params.Position)
	tok, ts, te := tokenAt(text, off)
	if tok == "" {
		return nil, nil
	}
	snap := s.session.snapshot()
	if snap == nil {
		return nil, nil
	}
	// account?
	if strings.Contains(tok, ":") {
		ledger := s.session.resolveLedger(path)
		if info, ok := snap.Accounts[ledger][tok]; ok {
			mc := &protocol.MarkupContent{Kind: protocol.MarkupKindPlainText, Value: fmtAccountHover(info)}
			return &protocol.Hover{Contents: mc, Range: rngPtr(rangeFromBytes(text, ts, te, nil))}, nil
		}
		// unknown account — thin signal
		mc := &protocol.MarkupContent{Kind: protocol.MarkupKindPlainText, Value: fmt.Sprintf("account %s\n(not opened in ledger %s)", tok, ledger)}
		return &protocol.Hover{Contents: mc, Range: rngPtr(rangeFromBytes(text, ts, te, nil))}, nil
	}
	// commodity
	pol := config.PolicyFor(snap.Policies, tok)
	info, has := snap.Commodities[tok]
	if !has {
		// still show policy if in CUE
		if _, ok := snap.Policies[tok]; !ok {
			return nil, nil
		}
		info = engine.CommodityInfo{Currency: tok}
	}
	mc := &protocol.MarkupContent{Kind: protocol.MarkupKindPlainText, Value: fmtCommodityHover(tok, info, pol)}
	return &protocol.Hover{Contents: mc, Range: rngPtr(rangeFromBytes(text, ts, te, nil))}, nil
}

// Ensure server type satisfies protocol.Server at compile time.
func rngPtr(r protocol.Range) *protocol.Range { return &r }

var _ protocol.Server = (*Server)(nil)

// silence unused imports if version path changes
var (
	_ = filepath.Separator
	_ = slog.Info
	_ = uri.File
)
