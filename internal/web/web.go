package web

//go:generate go tool templ generate
//go:generate go tool tailwind -i input.css -o static/app.css

import (
	"cmp"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cuelang.org/go/cue"
	"github.com/a-h/templ"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
	docsutil "github.com/lucasew/contapila-go/internal/docs"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/period"
	"github.com/lucasew/contapila-go/internal/prices"
	"github.com/lucasew/contapila-go/pkg/project"
)

//go:embed all:static
var staticFS embed.FS

// Sentinel errors for the web server.
var (
	ErrProjectRootRequired = errors.New("web: project root is required")
	ErrInvalidAsOfDate     = errors.New("invalid as-of date (want YYYY-MM-DD)")
)

type Server struct {
	// Root is the project directory (contapila.cue). Config, prices, ledger
	// discovery, and journals are reloaded from disk on every request so F5
	// always reflects current files — no process-lifetime cache of project state.
	Root string
	// Pages is the ledger report table (sidebar + /l/{ledger}/{page} + build).
	// Nil means DefaultPages() builtins.
	Pages *PageRegistry
}

// Listen serves the web UI until ctx is canceled or the process receives
// SIGINT/SIGTERM. Callers should pass cmd.Context() from cobra.
func Listen(ctx context.Context, p *project.Project, pdb *prices.DB, defaultLedger string, addr string) error {
	s, err := New(p, pdb)
	if err != nil {
		return err
	}
	fmt.Printf("contapila web on http://%s/\n", addr)
	if defaultLedger != "" {
		fmt.Printf("  ledger: http://%s/l/%s/check\n", addr, defaultLedger)
	}
	// Explicit timeouts: bare ListenAndServe has none (slowloris / hung conns).
	// Values sized for a local read-only ledger UI, not a high-traffic public API.
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful stop on SIGINT/SIGTERM so unit restarts drain in-flight requests.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		// Parent is already canceled; WithoutCancel so the timeout can run.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("web shutdown: %w", err)
		}
		return <-errCh
	case err := <-errCh:
		return err
	}
}

// New builds a web server rooted at p.Root. The prices argument is kept for
// call-site compatibility. Live handlers use a new Session per request (see
// withSession); project/ledger data is not cached on Server.
func New(p *project.Project, _ *prices.DB) (*Server, error) {
	if p == nil || p.Root == "" {
		return nil, ErrProjectRootRequired
	}
	return &Server{Root: p.Root}, nil
}

// withSecurityHeaders sets baseline browser hardening headers on every response.
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// Layout uses small inline scripts for theme bootstrap; static charts load from /static.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// withSession attaches a cold Session for this request when the context does
// not already carry one (build pre-attaches a shared Session).
func (s *Server) withSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sessionFrom(r.Context()) == nil {
			r = r.WithContext(withSession(r.Context(), NewSession(s.Root)))
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	DefaultRegistry(s).Mount(mux)
	return withSecurityHeaders(s.withSession(mux))
}

// PageData is the render model for ledger reports. First-party plugins outside
// package web use this type for Page.Fill / Page.Body.
type PageData = pageData

type pageData struct {
	// Sess drives link URL shape (live vs static .html) and is the load Session.
	Sess *Session
	// Pages selects sidebar/body for this render; nil means DefaultPages().
	Pages        *PageRegistry
	Title        string
	Page         string
	LedgerName   string
	Ledgers      []string
	ProjectRoot  string
	OpCurrency   string
	Diags        diag.List
	HasErrors    bool
	HasWarnings  bool
	OK           bool
	BalanceRows  []balanceRow
	Journal      []engine.JournalEntry
	IncomeRows   []balanceRow
	ExpenseRows  []balanceRow
	NetWorthRows []nwRow
	NetWorthTot  string
	AsOf         string
	Time         string
	PeriodLabel  string
	Error        string
	// Account page
	AccountName       string
	AccountBalances   []balanceRow
	AccountActivity   []balanceRow
	AccountDocs       []docRow
	AccountMeta       []metaKV
	AccountCurrencies []string
	// AccountList is the /accounts report (plugin web_accounts).
	AccountList []AccountListRow
	// EventList is the /events report (plugin web_events).
	EventList []EventListRow
	// QueryNav is named journal queries for the sidebar (any page).
	QueryNav []QueryNavItem
	// JumpAccounts / JumpCommodities feed the command palette (sorted keys).
	JumpAccounts    []string
	JumpCommodities []string
	// PluginCommands are extra palette rows from plugin.Reg.Palette (enabled modules).
	PluginCommands []CommandItem
	// QueryList is the /queries report (plugin web_queries).
	QueryList []QueryListRow
	// Query detail page (/l/{ledger}/query/{name}).
	QueryName string
	QueryText string // plain BQL; highlighted in templ via codeHighlight
	// Documents is the ledger documents report (/documents) list.
	Documents []docRow
	// Prices is the shared price-pairs report (/prices).
	PriceSeries []priceSeriesRow
	// Commodity page
	CommodityName     string
	CommodityBalances []balanceRow // Account + Amount for this commodity
	CommodityActivity []balanceRow
	CommodityMeta     []metaKV
	CommodityPrices   []pricePointRow // price history for this base commodity
	// Charts (uPlot): ChartJSON is pre-marshaled JSON embedded in the page.
	ChartID    string
	ChartTitle string
	ChartJSON  string
	NeedCharts bool
	// Debug page (core): plugin enablement + CUE dump.
	PluginRows []PluginStatusRow
	PluginsCUE string // plain plugins subtree (codeHighlight at render)
	ConfigCUE  string // plain full config (codeHighlight at render)
}

// PluginStatusRow is one registered module on the debug page.
type PluginStatusRow struct {
	ID        string
	Enabled   bool
	InConfig  bool // concrete entry under plugins after unify
	HasStream bool
	EntryCUE  string // plain plugins.<id> fragment (codeHighlight at render)
}

// AccountListRow is one open account on the accounts report.
type AccountListRow struct {
	Account    string
	OpenDate   string
	Currencies []string
}

// EventListRow is one journal event on the events report.
type EventListRow struct {
	Date string
	Type string
	Desc string
}

// QueryNavItem is one named query in the sidebar.
type QueryNavItem struct {
	Name string
}

// QueryListRow is one journal query on the queries report.
type QueryListRow struct {
	Date string
	Name string
	Text string
}

// priceSeriesRow is one base/quote pair summary on the prices report.
type priceSeriesRow struct {
	Base, Quote string
	Count       int
	FirstDate   string
	LastDate    string
	LastRate    string
	LastMeta    []metaKV
}

// pricePointRow is one observation on a commodity's price history.
type pricePointRow struct {
	Date     string
	Quote    string
	Rate     string
	Metadata []metaKV
}

type metaKV struct {
	Key, Value string
}

type docRow struct {
	Date      string
	Account   string // owning account (for links)
	TreePath  string // collapse key (account, or account+\x1f+file path)
	Name      string // leaf label: account segment or filename
	Path      string // project-relative file path
	Href      string // /docfile/... URL
	FileName  string // base filename
	Synthetic bool
	Depth     int
	IsRollup  bool
	IsDoc     bool // document leaf under an account
	PadLeft   string
}

type balanceRow struct {
	Account   string
	Path      string // collapse key (account or account+\x1f+commodity)
	Name      string // short label (last segment) for tree display
	Commodity string
	Amount    string
	Depth     int    // hierarchical indent
	IsRollup  bool   // parent total row
	PadLeft   string // e.g. "1.5rem" for template indent
}

type nwRow struct {
	Account   string
	Path      string // collapse key
	Name      string // leaf segment
	Commodity string
	Units     string
	Value     string
	Unpriced  bool
	Depth     int
	IsRollup  bool
	PadLeft   string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())
	if sess == nil {
		sess = NewSession(s.Root)
	}
	p, _, err := sess.Project(r.Context())
	if err != nil {
		slog.Error("web handler", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	data := pageData{
		Sess:        sess,
		Title:       "Ledgers",
		Page:        "home",
		Ledgers:     engine.LedgerNames(p),
		ProjectRoot: p.Root,
	}
	s.render(w, r, IndexPage(data))
}

// pageTimeQuery is shared time-filter + as-of resolution for ledger/account/commodity pages.
type pageTimeQuery struct {
	Time        string
	Period      period.Range
	PeriodErr   error
	PeriodLabel string
	AsOf        time.Time
	AsOfStr     string
}

// parsePageTimeQuery reads time (and optionally from/to, as-of) from q.
// fromTo: compose time from from/to when time is empty.
// explicitAsOf: honor as-of with validation (invalid → error).
// Ledger, account, and commodity handlers pass fromTo=true, explicitAsOf=true.
// Invalid explicit as-of returns a non-nil error; period parse errors stay in PeriodErr.
func parsePageTimeQuery(q url.Values, now time.Time, fromTo, explicitAsOf bool) (pageTimeQuery, error) {
	timeStr := q.Get("time")
	if timeStr == "" && fromTo {
		fromStr, toStr := q.Get("from"), q.Get("to")
		switch {
		case fromStr != "" && toStr != "":
			timeStr = fromStr + " - " + toStr
		case fromStr != "":
			timeStr = fromStr
		case toStr != "":
			timeStr = toStr
		}
	}

	pr, perr := period.Parse(timeStr, now)
	out := pageTimeQuery{
		Time:        timeStr,
		Period:      pr,
		PeriodErr:   perr,
		PeriodLabel: period.DisplayLabel(timeStr, now),
	}

	// as-of: explicit flag wins (when enabled); else end of time filter; else far future
	if explicitAsOf {
		asOfStr := q.Get("as-of")
		if asOfStr != "" {
			parsed, err := engine.ParseDate(asOfStr)
			if err != nil {
				return out, fmt.Errorf("%w: %s", ErrInvalidAsOfDate, asOfStr)
			}
			out.AsOf = parsed
			out.AsOfStr = asOfStr
			return out, nil
		}
	}
	if !pr.End.IsZero() {
		out.AsOf = pr.End
		out.AsOfStr = pr.End.Format("2006-01-02")
	} else {
		out.AsOf = engine.AsOfLatest
		out.AsOfStr = ""
	}
	return out, nil
}

func (s *Server) handleLedgerPage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("ledger")
	pageID := r.PathValue("page")
	proj, pdb, l, tq, sess, ok := s.openLedgerRequest(w, r, name)
	if !ok {
		return
	}
	reg := s.resolvedPagesFor(r.Context(), sess, l)
	pg, ok := reg.Lookup(pageID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	title := name + " · " + pg.Label
	if pg.Label == "" {
		title = name + " · " + pageID
	}
	data := s.basePageData(r.Context(), sess, proj, l, name, title, pageID, tq)
	data.Pages = reg
	ds := l.Check()
	data.Diags = ds
	data.HasErrors = ds.HasErrors()
	data.HasWarnings = ds.HasWarnings()
	data.OK = !ds.HasErrors()

	if pg.Fill != nil {
		pg.Fill(PageContext{
			Request:    r,
			Sess:       sess,
			Project:    proj,
			Prices:     pdb,
			Ledger:     l,
			LedgerName: name,
			Time:       tq.Time,
			TimeQuery:  tq,
			Period:     tq.Period,
			PeriodErr:  tq.PeriodErr,
			AsOf:       tq.AsOf,
		}, &data)
	}
	s.render(w, r, LedgerPage(data))
}

func (s *Server) handleAccount(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("ledger")
	account, err := url.PathUnescape(r.PathValue("account"))
	if err != nil {
		http.Error(w, "invalid account path encoding", http.StatusBadRequest)
		return
	}
	if account == "" {
		http.NotFound(w, r)
		return
	}
	proj, _, l, tq, sess, ok := s.openLedgerRequest(w, r, name)
	if !ok {
		return
	}
	pr, perr, asOf := tq.Period, tq.PeriodErr, tq.AsOf

	data := s.basePageData(r.Context(), sess, proj, l, name, account, "account", tq)
	data.AccountName = account
	if perr != nil {
		s.render(w, r, AccountPage(data))
		return
	}

	data.AccountBalances = amountMapRows(l.AccountBalances(account, asOf), account, "", 4)
	data.AccountActivity = amountMapRows(l.AccountActivity(account, pr.Start, pr.End), account, "", 4)
	data.Journal = l.JournalForAccount(account, pr.Start, pr.End)
	data.AccountDocs = documentRows(l.DocumentsForAccount(account))
	if info, ok := l.Account(account); ok {
		data.AccountMeta = metaRows(info.Metadata)
		data.AccountCurrencies = info.Currencies
	}
	if pts, err := l.AccountSeries(account, pr.Start, pr.End); err == nil {
		if js, jerr := chartLineJSON(pts, l.OpCurrency, "Balance"); jerr == nil {
			setChart(&data, "chart-account", "Balance over time", js)
		}
	}
	s.render(w, r, AccountPage(data))
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	ledger := r.PathValue("ledger")
	qname, err := url.PathUnescape(r.PathValue("name"))
	if err != nil {
		http.Error(w, "invalid query path encoding", http.StatusBadRequest)
		return
	}
	if qname == "" {
		http.NotFound(w, r)
		return
	}
	proj, _, l, tq, sess, ok := s.openLedgerRequest(w, r, ledger)
	if !ok {
		return
	}
	// Opt-in UI: only when web_queries is in the resolved page table.
	reg := s.resolvedPagesFor(r.Context(), sess, l)
	if _, enabled := reg.Lookup("queries"); !enabled {
		http.NotFound(w, r)
		return
	}
	var found bool
	var text string
	for _, q := range l.Queries() {
		if q.Name == qname {
			found = true
			text = q.Query
			break
		}
	}
	title := qname
	data := s.basePageData(r.Context(), sess, proj, l, ledger, title, "query", tq)
	data.Pages = reg
	data.QueryName = qname
	data.QueryText = text
	if !found {
		data.Error = "query not found: " + qname
	}
	s.render(w, r, QueryPage(data))
}

// chartLineJSON builds uPlot line payload (event series, op currency).
func chartLineJSON(pts []engine.SeriesPoint, currency, label string) (string, error) {
	if len(pts) == 0 {
		return "", nil
	}
	type payload struct {
		Kind     string    `json:"kind"`
		Currency string    `json:"currency"`
		Label    string    `json:"label"`
		X        []int64   `json:"x"`
		Y        []float64 `json:"y"`
	}
	p := payload{Kind: "line", Currency: currency, Label: label, X: make([]int64, len(pts)), Y: make([]float64, len(pts))}
	for i, pt := range pts {
		p.X[i] = pt.Date.UTC().Unix()
		p.Y[i] = ratFloat(pt.Value)
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// chartPriceJSON builds a stepped line of market prices for base commodity.
// Prefers preferQuote (usually op currency); otherwise first series by quote name.
// Points outside [from,to] are dropped when bounds are set; zero bounds = full series.
// If the filter removes every point, falls back to the full series so the chart still shows.
func chartPriceJSON(db *prices.DB, base, preferQuote string, from, to time.Time) (string, string, error) {
	if db == nil || base == "" {
		return "", "", nil
	}
	series := db.SeriesForBase(base)
	if len(series) == 0 {
		return "", "", nil
	}
	chosen := series[0]
	if preferQuote != "" {
		for _, s := range series {
			if s.Quote == preferQuote {
				chosen = s
				break
			}
		}
	}
	type payload struct {
		Kind     string    `json:"kind"`
		Currency string    `json:"currency"`
		Label    string    `json:"label"`
		Height   int       `json:"height,omitempty"`
		X        []int64   `json:"x"`
		Y        []float64 `json:"y"`
	}
	build := func(filter bool) payload {
		p := payload{
			Kind:     "line",
			Currency: chosen.Quote,
			Label:    base + "/" + chosen.Quote,
			Height:   220,
		}
		fromD, toD := from, to
		if filter {
			if !fromD.IsZero() {
				fromD = time.Date(fromD.Year(), fromD.Month(), fromD.Day(), 0, 0, 0, 0, time.UTC)
			}
			if !toD.IsZero() {
				toD = time.Date(toD.Year(), toD.Month(), toD.Day(), 0, 0, 0, 0, time.UTC)
			}
		} else {
			fromD, toD = time.Time{}, time.Time{}
		}
		for _, pt := range chosen.Points {
			if pt.Rate == nil {
				continue
			}
			d := time.Date(pt.Date.Year(), pt.Date.Month(), pt.Date.Day(), 0, 0, 0, 0, time.UTC)
			if !fromD.IsZero() && d.Before(fromD) {
				continue
			}
			if !toD.IsZero() && d.After(toD) {
				continue
			}
			p.X = append(p.X, d.Unix())
			p.Y = append(p.Y, ratFloat(pt.Rate))
		}
		return p
	}
	p := build(true)
	if len(p.X) == 0 && (!from.IsZero() || !to.IsZero()) {
		p = build(false)
	}
	if len(p.X) == 0 {
		return "", "", nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", "", err
	}
	return string(b), chosen.Quote, nil
}

// chartBarsJSON builds uPlot diverging bar payload.
// X is ordinal (0..n-1), not unix time — avoids time-scale bar width/overlap artifacts.
func chartBarsJSON(bars []engine.BarPoint, currency string) (string, error) {
	if len(bars) == 0 {
		return "", nil
	}
	type payload struct {
		Kind     string    `json:"kind"`
		Currency string    `json:"currency"`
		X        []float64 `json:"x"`
		Labels   []string  `json:"labels"`
		Income   []float64 `json:"income"`
		Expense  []float64 `json:"expense"`
	}
	p := payload{
		Kind: "bars", Currency: currency,
		X: make([]float64, len(bars)), Labels: make([]string, len(bars)),
		Income: make([]float64, len(bars)), Expense: make([]float64, len(bars)),
	}
	for i, b := range bars {
		p.X[i] = float64(i)
		p.Labels[i] = b.Label
		// Per-bin flow magnitudes (not cumulative across bins).
		p.Income[i] = ratFloat(b.Income)
		p.Expense[i] = ratFloat(b.Expense)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func ratFloat(r *big.Rat) float64 {
	if r == nil {
		return 0
	}
	f, exact := r.Float64()
	_ = exact // chart series is float64 by design
	return f
}

func documentRows(docs []ast.Document) []docRow {
	var rows []docRow
	for _, d := range docs {
		p := filepath.ToSlash(d.Path)
		p = strings.TrimPrefix(p, "/")
		href := ""
		// serve only <ledger>/docs/... via /docfile/
		if docsutil.IsLedgerDocPath(p) {
			href = "/docfile/" + p
		}
		rows = append(rows, docRow{
			Date:      d.Date.Format("2006-01-02"),
			Account:   d.Account,
			TreePath:  d.Account,
			Path:      p,
			Href:      href,
			Name:      path.Base(p),
			FileName:  path.Base(p),
			Synthetic: d.Synthetic,
		})
	}
	return rows
}

// documentTreeRows builds a collapsible account hierarchy for the documents report
// (same collapse semantics as balances / net worth / P&L).
func documentTreeRows(docs []ast.Document) []docRow {
	if len(docs) == 0 {
		return nil
	}
	type fileInfo struct {
		date      string
		path      string
		href      string
		name      string
		synthetic bool
	}
	byAcct := map[string][]fileInfo{}
	var noAcct []fileInfo
	for _, d := range docs {
		p := filepath.ToSlash(d.Path)
		p = strings.TrimPrefix(p, "/")
		href := ""
		if docsutil.IsLedgerDocPath(p) {
			href = "/docfile/" + p
		}
		fi := fileInfo{
			date:      d.Date.Format("2006-01-02"),
			path:      p,
			href:      href,
			name:      path.Base(p),
			synthetic: d.Synthetic,
		}
		if d.Account == "" {
			noAcct = append(noAcct, fi)
			continue
		}
		byAcct[d.Account] = append(byAcct[d.Account], fi)
	}
	for a := range byAcct {
		// Newest first within each account.
		sort.SliceStable(byAcct[a], func(i, j int) bool {
			if byAcct[a][i].date != byAcct[a][j].date {
				return byAcct[a][i].date > byAcct[a][j].date
			}
			return byAcct[a][i].path < byAcct[a][j].path
		})
	}

	leaves := make([]string, 0, len(byAcct))
	for a := range byAcct {
		leaves = append(leaves, a)
	}
	tree := engine.NewAccountTree(leaves)

	hasDocsUnder := map[string]bool{}
	for _, n := range tree.Names {
		for a, list := range byAcct {
			if len(list) == 0 {
				continue
			}
			if a == n || strings.HasPrefix(a, n+":") {
				hasDocsUnder[n] = true
				break
			}
		}
	}

	out := make([]docRow, 0, len(tree.Names)+len(docs))
	// Unassigned docs first (flat).
	for _, fi := range noAcct {
		out = append(out, docRow{
			Date:      fi.date,
			TreePath:  engine.TreePathSep + fi.path,
			Name:      fi.name,
			Path:      fi.path,
			Href:      fi.href,
			FileName:  fi.name,
			Synthetic: fi.synthetic,
			IsDoc:     true,
		})
	}

	for _, n := range tree.Names {
		if !hasDocsUnder[n] {
			continue
		}
		own := byAcct[n]
		depth := strings.Count(n, ":")
		leaf := n
		if i := strings.LastIndex(n, ":"); i >= 0 && i+1 < len(n) {
			leaf = n[i+1:]
		}
		child := tree.HasChild[n]

		// Single doc, no sub-accounts → one flat leaf row (like single-commodity balance).
		if !child && len(own) == 1 {
			fi := own[0]
			out = append(out, docRow{
				Date:      fi.date,
				Account:   n,
				TreePath:  n,
				Name:      leaf,
				Path:      fi.path,
				Href:      fi.href,
				FileName:  fi.name,
				Synthetic: fi.synthetic,
				Depth:     depth,
				PadLeft:   treePadLeft(depth),
				IsDoc:     false, // show account name; file in File column
			})
			continue
		}

		// Parent or multi-doc leaf: rollup account row, then document children if any on this node.
		isRollup := child || len(own) > 0
		out = append(out, docRow{
			Account:  n,
			TreePath: n,
			Name:     leaf,
			Depth:    depth,
			IsRollup: isRollup,
			PadLeft:  treePadLeft(depth),
		})
		if len(own) == 0 {
			continue
		}
		// Always list docs under the account when it's a multi-doc leaf or has children.
		// (Single-doc leaf already returned above.)
		for _, fi := range own {
			out = append(out, docRow{
				Date:      fi.date,
				Account:   n,
				TreePath:  n + engine.TreePathSep + fi.path,
				Name:      fi.name,
				Path:      fi.path,
				Href:      fi.href,
				FileName:  fi.name,
				Synthetic: fi.synthetic,
				Depth:     depth + 1,
				PadLeft:   treePadLeft(depth + 1),
				IsDoc:     true,
			})
		}
	}
	return out
}

// handleDocFile serves project-relative paths under <ledger>/docs/ only.
// OpenRoot scopes opens to the project root so path traversal cannot escape.
func (s *Server) handleDocFile(w http.ResponseWriter, r *http.Request) {
	rel := filepath.ToSlash(r.PathValue("path"))
	rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
	if !docsutil.IsLedgerDocPath(rel) {
		http.NotFound(w, r)
		return
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	f, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

func (s *Server) handleCommodity(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("ledger")
	commodity, err := url.PathUnescape(r.PathValue("commodity"))
	if err != nil {
		http.Error(w, "invalid commodity path encoding", http.StatusBadRequest)
		return
	}
	if commodity == "" {
		http.NotFound(w, r)
		return
	}
	proj, pdb, l, tq, sess, ok := s.openLedgerRequest(w, r, name)
	if !ok {
		return
	}
	pr, perr, asOf := tq.Period, tq.PeriodErr, tq.AsOf

	data := s.basePageData(r.Context(), sess, proj, l, name, commodity, "commodity", tq)
	data.CommodityName = commodity
	if perr != nil {
		s.render(w, r, CommodityPage(data))
		return
	}

	data.CommodityBalances = amountMapRows(l.CommodityBalances(commodity, asOf), "", commodity, 4)
	data.CommodityActivity = amountMapRows(l.CommodityActivity(commodity, pr.Start, pr.End), "", commodity, 4)
	data.Journal = l.JournalForCommodity(commodity, pr.Start, pr.End)
	if info, ok := l.Commodities[commodity]; ok {
		data.CommodityMeta = metaRows(info.Metadata)
	}
	// Overlay CUE commodity fields (name, asset-class, …) when present.
	if proj.Config != nil {
		data.CommodityMeta = mergeCUECommodityMeta(proj.Config.Value, commodity, data.CommodityMeta)
	}
	data.CommodityPrices = commodityPriceRows(pdb, commodity)
	// Price chart uses full history (not the journal time filter). Filtering prices
	// by "month" / current year often empties the chart even when history exists.
	if js, quote, jerr := chartPriceJSON(pdb, commodity, l.OpCurrency, time.Time{}, time.Time{}); jerr == nil {
		title := "Price"
		if quote != "" {
			title = "Price (" + quote + ")"
		}
		setChart(&data, "chart-commodity-price", title, js)
	}
	s.render(w, r, CommodityPage(data))
}

func priceSeriesRows(db *prices.DB) []priceSeriesRow {
	if db == nil {
		return nil
	}
	series := db.AllSeries()
	out := make([]priceSeriesRow, 0, len(series))
	for _, s := range series {
		if len(s.Points) == 0 {
			continue
		}
		first, last := s.Points[0], s.Points[len(s.Points)-1]
		row := priceSeriesRow{
			Base:      s.Base,
			Quote:     s.Quote,
			Count:     len(s.Points),
			FirstDate: first.Date.Format("2006-01-02"),
			LastDate:  last.Date.Format("2006-01-02"),
			LastRate:  last.Rate.FloatString(6),
			LastMeta:  metaRows(last.Metadata),
		}
		out = append(out, row)
	}
	return out
}

func commodityPriceRows(db *prices.DB, base string) []pricePointRow {
	if db == nil {
		return nil
	}
	var out []pricePointRow
	for _, s := range db.SeriesForBase(base) {
		// newest first for history reading
		for i := len(s.Points) - 1; i >= 0; i-- {
			p := s.Points[i]
			out = append(out, pricePointRow{
				Date:     p.Date.Format("2006-01-02"),
				Quote:    s.Quote,
				Rate:     p.Rate.FloatString(6),
				Metadata: metaRows(p.Metadata),
			})
		}
	}
	return out
}

func metaRows(m ast.Metadata) []metaKV {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]metaKV, 0, len(keys))
	for _, k := range keys {
		out = append(out, metaKV{Key: k, Value: m[k]})
	}
	return out
}

// mergeCUECommodityMeta adds missing keys from contapila.cue commodities.<name>.
func mergeCUECommodityMeta(cfg cue.Value, commodity string, rows []metaKV) []metaKV {
	if !cfg.Exists() {
		return rows
	}
	// commodities."B3_PETR4" path for odd names
	v := cfg.LookupPath(cue.ParsePath("commodities." + strconv.Quote(commodity)))
	if !v.Exists() {
		v = cfg.LookupPath(cue.ParsePath("commodities." + commodity))
	}
	if !v.Exists() {
		return rows
	}
	have := map[string]bool{}
	for _, r := range rows {
		have[r.Key] = true
	}
	it, err := v.Fields()
	if err != nil {
		return rows
	}
	var extra []metaKV
	for it.Next() {
		sel := it.Selector()
		k := sel.String()
		if k == "precision" || have[k] {
			continue
		}
		s, err := it.Value().String()
		if err != nil {
			// allow int precision already skipped; try other concrete forms
			continue
		}
		extra = append(extra, metaKV{Key: k, Value: s})
		have[k] = true
	}
	if len(extra) == 0 {
		return rows
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i].Key < extra[j].Key })
	return append(rows, extra...)
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := c.Render(r.Context(), w); err != nil {
		slog.Error("web render", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// treePadStepRem is the CSS padding-left increment per tree depth level.
const treePadStepRem = 0.75

// treePadLeft is CSS padding-left for hierarchical tree rows (treePadStepRem per depth).
func treePadLeft(depth int) string {
	if depth <= 0 {
		return ""
	}
	return strconv.FormatFloat(float64(depth)*treePadStepRem, 'f', 2, 64) + "rem"
}

func treeLabel(name, account string) string {
	return cmp.Or(name, account)
}

func treePath(path, account string) string {
	return cmp.Or(path, account)
}

func buildBalanceTreeRows(lines []engine.BalanceTreeLine) []balanceRow {
	rows := make([]balanceRow, 0, len(lines))
	for _, ln := range lines {
		amt := ""
		if ln.Amount != nil {
			amt = ln.Amount.FloatString(4)
		}
		rows = append(rows, balanceRow{
			Account:   ln.Account,
			Path:      treePath(ln.Path, ln.Account),
			Name:      treeLabel(ln.Name, ln.Account),
			Commodity: ln.Commodity,
			Amount:    amt,
			Depth:     ln.Depth,
			IsRollup:  ln.IsRollup,
			PadLeft:   treePadLeft(ln.Depth),
		})
	}
	return rows
}

func buildPnLRows(lines []engine.PnLLine) []balanceRow {
	rows := make([]balanceRow, 0, len(lines))
	for _, ln := range lines {
		amt := ""
		if ln.Amount != nil {
			amt = ln.Amount.FloatString(2)
		}
		rows = append(rows, balanceRow{
			Account:   ln.Account,
			Name:      treeLabel(ln.Name, ln.Account),
			Commodity: ln.Commodity,
			Amount:    amt,
			Depth:     ln.Depth,
			IsRollup:  ln.IsRollup,
			PadLeft:   treePadLeft(ln.Depth),
		})
	}
	return rows
}

func buildNetWorthRows(lines []engine.NetWorthTreeLine) []nwRow {
	rows := make([]nwRow, 0, len(lines))
	for _, ln := range lines {
		units := ""
		if ln.Units != nil {
			units = ln.Units.FloatString(4)
		}
		val := ""
		if ln.Value != nil {
			val = ln.Value.FloatString(2)
		}
		rows = append(rows, nwRow{
			Account:   ln.Account,
			Path:      treePath(ln.Path, ln.Account),
			Name:      treeLabel(ln.Name, ln.Account),
			Commodity: ln.Commodity,
			Units:     units,
			Value:     val,
			Unpriced:  ln.Unpriced,
			Depth:     ln.Depth,
			IsRollup:  ln.IsRollup,
			PadLeft:   treePadLeft(ln.Depth),
		})
	}
	return rows
}
