package lsp

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/diag"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/filesys"
	"github.com/lucasew/contapila-go/internal/parser"
	"github.com/lucasew/contapila-go/internal/source"
	"github.com/lucasew/contapila-go/pkg/project"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

const debounceDelay = 300 * time.Millisecond

// Snapshot is a last-good project index + semantic diagnostics.
type Snapshot struct {
	Root        string
	Accounts    map[string]map[string]engine.AccountInfo // ledger -> account -> info
	Commodities map[string]engine.CommodityInfo          // project-shared
	Policies    map[string]config.CommodityPolicy
	// Diags by absolute path (check + load).
	Diags map[string][]protocol.Diagnostic
	// Ledger owning each open document path (best-effort).
	PathLedger map[string]string
}

// Session holds overlays, parse diags, and the last-good snapshot.
type Session struct {
	mu sync.RWMutex

	overlay *filesys.Overlay
	// openDocs: abs path -> version
	openDocs map[string]int32
	// parseDiags: abs path -> diagnostics from last parse of that buffer
	parseDiags map[string][]protocol.Diagnostic
	// docText cache from overlay is enough; keep for closed? no

	snap *Snapshot

	// first project root wins
	root string

	client protocol.Client

	// rebuild scheduling
	dirty     bool
	timer     *time.Timer
	cancelBld context.CancelFunc
	gen       uint64
}

func newSession() *Session {
	return &Session{
		overlay:    filesys.NewOverlay(nil),
		openDocs:   map[string]int32{},
		parseDiags: map[string][]protocol.Diagnostic{},
		snap: &Snapshot{
			Accounts:    map[string]map[string]engine.AccountInfo{},
			Commodities: map[string]engine.CommodityInfo{},
			Policies:    map[string]config.CommodityPolicy{},
			Diags:       map[string][]protocol.Diagnostic{},
			PathLedger:  map[string]string{},
		},
	}
}

func (s *Session) setClient(c protocol.Client) {
	s.mu.Lock()
	s.client = c
	s.mu.Unlock()
}

func (s *Session) ensureRoot(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root != "" {
		return
	}
	dir := path
	if !isDir(path) {
		dir = filepath.Dir(path)
	}
	// walk using disk (marker usually not overlayed)
	p, err := project.OpenProject(dir)
	if err != nil {
		return
	}
	s.root = p.Root
}

func isDir(path string) bool {
	// heuristic: no extension often dir — use Stat
	info, err := filesys.OS{}.Stat(path)
	return err == nil && info.IsDir()
}

func (s *Session) markDirty() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(debounceDelay, func() {
		s.scheduleRebuild()
	})
}

func (s *Session) scheduleRebuild() {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return
	}
	s.dirty = false
	if s.cancelBld != nil {
		s.cancelBld()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelBld = cancel
	s.gen++
	gen := s.gen
	client := s.client
	s.mu.Unlock()

	go s.rebuild(ctx, gen, client)
}

func (s *Session) rebuild(ctx context.Context, gen uint64, client protocol.Client) {
	s.mu.RLock()
	root := s.root
	s.mu.RUnlock()
	if root == "" {
		// try from any open doc
		s.mu.RLock()
		for p := range s.openDocs {
			s.mu.RUnlock()
			s.ensureRoot(p)
			s.mu.RLock()
			root = s.root
			break
		}
		s.mu.RUnlock()
	}
	if root == "" {
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}

	fsys := s.overlay
	p, pdb, pdiags, err := engine.OpenProjectFS(fsys, root)
	if err != nil {
		slog.Debug("lsp rebuild: open project", "err", err)
		if client != nil {
			_ = client.LogMessage(ctx, &protocol.LogMessageParams{
				Type:    protocol.MessageTypeWarning,
				Message: "contapila: " + err.Error(),
			})
		}
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}

	// Parse-pass gate: require open docs (if any) to parse without hard error.
	// We still open ledgers; if a ledger load fails, skip snapshot swap for indexes
	// but that's handled per-ledger.
	snap := &Snapshot{
		Root:        p.Root,
		Accounts:    map[string]map[string]engine.AccountInfo{},
		Commodities: map[string]engine.CommodityInfo{},
		Policies:    map[string]config.CommodityPolicy{},
		Diags:       map[string][]protocol.Diagnostic{},
		PathLedger:  map[string]string{},
	}
	if p.Config != nil {
		snap.Policies = config.CommodityPolicies(p.Config.Value)
	}

	// Seed path→ledger from mains.
	for _, l := range p.Ledgers {
		snap.PathLedger[l.MainPath] = l.Name
	}

	// project open diags
	mergeDiags(snap.Diags, pdiags, "")

	// Which ledgers have open files?
	openLedgers := map[string]bool{}
	s.mu.RLock()
	for path := range s.openDocs {
		name := ledgerForPath(p, path)
		if name != "" {
			openLedgers[name] = true
			snap.PathLedger[path] = name
		}
	}
	// If none mapped, open all (still dogfood-friendly for single-ledger projects).
	if len(openLedgers) == 0 {
		for _, l := range p.Ledgers {
			openLedgers[l.Name] = true
		}
	}
	s.mu.RUnlock()

	parseOK := true
	for name := range openLedgers {
		if err := ctx.Err(); err != nil {
			return
		}
		led, err := engine.OpenLedgerFS(fsys, p, pdb, name)
		if err != nil {
			slog.Debug("lsp rebuild: open ledger", "ledger", name, "err", err)
			parseOK = false
			continue
		}
		// collect accounts
		snap.Accounts[name] = led.Accounts
		for c, info := range led.Commodities {
			snap.Commodities[c] = info
		}
		// merge commodity from CUE-only keys already in policies
		mergeDiags(snap.Diags, led.Diags, "")
	}

	// Also pull commodities from prices stream via project prices path parse is in pdb load diags already.

	if !parseOK {
		// still allow partial? SPEC: only swap when parse passes.
		// If ledger open failed, do not swap indexes.
		return
	}

	s.mu.Lock()
	if gen != s.gen {
		s.mu.Unlock()
		return
	}
	s.snap = snap
	s.root = p.Root
	client = s.client
	// publish combined diags for open ledger files
	open := make(map[string]int32, len(s.openDocs))
	for k, v := range s.openDocs {
		open[k] = v
	}
	parseCopy := map[string][]protocol.Diagnostic{}
	for k, v := range s.parseDiags {
		parseCopy[k] = v
	}
	s.mu.Unlock()

	if client == nil {
		return
	}
	// Publish for every path that has semantic diags and is under open ledgers, plus open docs.
	published := map[string]bool{}
	for path, diags := range snap.Diags {
		if err := ctx.Err(); err != nil {
			return
		}
		// include parse diags for this path
		all := append([]protocol.Diagnostic{}, parseCopy[path]...)
		all = append(all, diags...)
		_ = client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
			URI:         pathToURI(path),
			Diagnostics: all,
		})
		published[path] = true
	}
	for path := range open {
		if published[path] {
			continue
		}
		_ = client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
			URI:         pathToURI(path),
			Diagnostics: parseCopy[path],
		})
	}
}

func ledgerForPath(p *project.Project, path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	// under <root>/<ledger>/
	rel, err := filepath.Rel(p.Root, abs)
	if err != nil {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return ""
	}
	cand := parts[0]
	for _, l := range p.Ledgers {
		if l.Name == cand {
			return l.Name
		}
	}
	return ""
}

func mergeDiags(dst map[string][]protocol.Diagnostic, list diag.List, _ string) {
	for _, d := range list {
		if d.File == "" {
			continue
		}
		abs, err := filepath.Abs(d.File)
		if err != nil {
			abs = d.File
		}
		// Need file text for ranges — use line-only when unknown.
		text := ""
		// try read later; for now line range with empty text still works
		var r protocol.Range
		if d.Line > 0 {
			r = lineRange(text, d.Line)
			if text == "" {
				// zero-width at line start
				r = protocol.Range{
					Start: protocol.Position{Line: uint32(d.Line - 1), Character: 0},
					End:   protocol.Position{Line: uint32(d.Line - 1), Character: 1},
				}
			}
		}
		sev := protocol.DiagnosticSeverityWarning
		if d.IsError() {
			sev = protocol.DiagnosticSeverityError
		}
		dst[abs] = append(dst[abs], protocol.Diagnostic{
			Range:    r,
			Severity: sev,
			Source:   protocol.NewOptional("contapila"),
			Message:  protocol.String(d.Message),
		})
	}
}

func pathToURI(path string) uri.URI {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return uri.File(abs)
}

func uriToPath(u uri.URI) string {
	// file:///path
	s := string(u)
	if strings.HasPrefix(s, "file://") {
		p := strings.TrimPrefix(s, "file://")
		// URI may be file:///home/...
		if strings.HasPrefix(p, "/") {
			return p
		}
		// Windows-ish skip
		return p
	}
	return s
}

// parseBuffer publishes parse diagnostics for one file; returns whether parse had errors.
func (s *Session) parseBuffer(path, text string) (hasErrors bool) {
	f := source.NewString(path, text)
	_, diags, err := parser.ParseFile(f)
	if err != nil {
		hasErrors = true
	}
	out := make([]protocol.Diagnostic, 0, len(diags))
	for _, d := range diags {
		if d.IsError() {
			hasErrors = true
		}
		sev := protocol.DiagnosticSeverityWarning
		if d.IsError() {
			sev = protocol.DiagnosticSeverityError
		}
		r := lineRange(text, d.Line)
		if d.Line <= 0 {
			r = protocol.Range{}
		}
		out = append(out, protocol.Diagnostic{
			Range:    r,
			Severity: sev,
			Source:   protocol.NewOptional("contapila"),
			Message:  protocol.String(d.Message),
		})
	}
	s.mu.Lock()
	s.parseDiags[path] = out
	client := s.client
	// merge with last-good semantic for this path when publishing parse-only
	var sem []protocol.Diagnostic
	if s.snap != nil {
		sem = s.snap.Diags[path]
	}
	s.mu.Unlock()

	if client != nil {
		all := append([]protocol.Diagnostic{}, out...)
		// SPEC: on parse failure, parse diags immediate; keep last-good check diags
		// On parse success we'll rebuild and swap — still show last-good until then.
		all = append(all, sem...)
		_ = client.PublishDiagnostics(context.Background(), &protocol.PublishDiagnosticsParams{
			URI:         pathToURI(path),
			Diagnostics: all,
		})
	}
	return hasErrors
}

func (s *Session) snapshot() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

func (s *Session) docText(path string) (string, bool) {
	return s.overlay.Get(path)
}

func (s *Session) ledgerOf(path string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.snap != nil {
		if n, ok := s.snap.PathLedger[path]; ok {
			return n
		}
		// try prefix
		if s.snap.Root != "" {
			// reconstruct
		}
	}
	return ""
}

func (s *Session) resolveLedger(path string) string {
	if n := s.ledgerOf(path); n != "" {
		return n
	}
	s.mu.RLock()
	root := s.root
	if s.snap != nil && s.snap.Root != "" {
		root = s.snap.Root
	}
	s.mu.RUnlock()
	if root == "" {
		return ""
	}
	// lightweight
	p, err := project.OpenProject(root)
	if err != nil {
		return ""
	}
	return ledgerForPath(p, path)
}

func fmtAccountHover(info engine.AccountInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "account %s\n", info.Account)
	if !info.OpenDate.IsZero() {
		fmt.Fprintf(&b, "open %s\n", info.OpenDate.Format("2006-01-02"))
	}
	if len(info.Currencies) > 0 {
		fmt.Fprintf(&b, "currencies: %s\n", strings.Join(info.Currencies, ", "))
	}
	for k, v := range info.Metadata {
		fmt.Fprintf(&b, "%s: %s\n", k, v)
	}
	return strings.TrimSpace(b.String())
}

func fmtCommodityHover(name string, info engine.CommodityInfo, pol config.CommodityPolicy) string {
	var b strings.Builder
	fmt.Fprintf(&b, "commodity %s\n", name)
	if !info.Date.IsZero() {
		fmt.Fprintf(&b, "declared %s\n", info.Date.Format("2006-01-02"))
	}
	fmt.Fprintf(&b, "precision: %d\n", pol.Precision)
	if pol.Tolerance != nil {
		fmt.Fprintf(&b, "tolerance: %s\n", pol.Tolerance.FloatString(12))
	}
	for k, v := range info.Metadata {
		fmt.Fprintf(&b, "%s: %s\n", k, v)
	}
	return strings.TrimSpace(b.String())
}
