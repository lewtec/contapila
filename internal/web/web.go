package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"cuelang.org/go/cue"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
	docsutil "github.com/lucasew/contapila-go/internal/docs"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/period"
	"github.com/lucasew/contapila-go/internal/prices"
	"github.com/lucasew/contapila-go/pkg/project"
)

//go:embed templates/*
var templateFS embed.FS

//go:embed all:static
var staticFS embed.FS

type Server struct {
	// Root is the project directory (contapila.cue). Config, prices, ledger
	// discovery, and journals are reloaded from disk on every request so F5
	// always reflects current files — no process-lifetime cache of project state.
	Root string
	Tmpl *template.Template
}

func Listen(p *project.Project, pdb *prices.DB, defaultLedger string, addr string) error {
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
	return srv.ListenAndServe()
}

// New builds a web server rooted at p.Root. The prices argument is kept for
// call-site compatibility; request handlers reload project + prices from disk
// via engine.OpenProject on every request (not cached on Server).
func New(p *project.Project, _ *prices.DB) (*Server, error) {
	funcMap := template.FuncMap{
		"eq":          func(a, b string) bool { return a == b },
		"queryEscape": url.QueryEscape,
		"jsonStr": func(s string) template.JS {
			b, err := json.Marshal(s)
			if err != nil {
				return template.JS(`""`)
			}
			return template.JS(b)
		},
		// dict builds a map for partials that need row + page fields, e.g.
		// {{template "tree-account-cell" dict "Row" . "Ledger" $.LedgerName "Time" $.Time}}
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, fmt.Errorf("dict: need even number of args")
			}
			m := make(map[string]any, len(kv)/2)
			for i := 0; i < len(kv); i += 2 {
				k, ok := kv[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key must be string")
				}
				m[k] = kv[i+1]
			}
			return m, nil
		},
		"ledgerURL": func(ledger, page, timeFilter string) string {
			u := "/l/" + url.PathEscape(ledger) + "/" + url.PathEscape(page)
			if timeFilter != "" {
				u += "?time=" + url.QueryEscape(timeFilter)
			}
			return u
		},
		"accountURL": func(ledger, account, timeFilter string) string {
			// Keep ":" readable in URLs; escape only reserved bits.
			u := "/l/" + url.PathEscape(ledger) + "/account/" + url.PathEscape(account)
			if timeFilter != "" {
				u += "?time=" + url.QueryEscape(timeFilter)
			}
			return u
		},
		"commodityURL": func(ledger, commodity, timeFilter string) string {
			u := "/l/" + url.PathEscape(ledger) + "/commodity/" + url.PathEscape(commodity)
			if timeFilter != "" {
				u += "?time=" + url.QueryEscape(timeFilter)
			}
			return u
		},
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	if p == nil || p.Root == "" {
		return nil, fmt.Errorf("web: project root is required")
	}
	return &Server{Root: p.Root, Tmpl: tmpl}, nil
}

// loadProject reloads contapila.cue, project_journals, prices, and ledger discovery.
func (s *Server) loadProject() (*project.Project, *prices.DB, error) {
	p, pdb, _, err := engine.OpenProject(s.Root)
	if err != nil {
		return nil, nil, err
	}
	return p, pdb, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// Built CSS (daisyUI themes via @plugin) — correct text/css MIME.
	mux.Handle("GET /static/", http.FileServer(http.FS(staticFS)))
	// Ledger docs: URL /docfile/<ledger>/docs/by-account/... → <root>/<ledger>/docs/...
	mux.HandleFunc("GET /docfile/{path...}", s.handleDocFile)
	mux.HandleFunc("GET /{$}", s.handleIndex)
	// Named entity routes before generic page so "account"/"commodity" are not page names.
	mux.HandleFunc("GET /l/{ledger}/account/{account...}", s.handleAccount)
	mux.HandleFunc("GET /l/{ledger}/commodity/{commodity...}", s.handleCommodity)
	mux.HandleFunc("GET /l/{ledger}/{page}", s.handleLedgerPage)
	mux.HandleFunc("GET /l/{ledger}/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/l/"+r.PathValue("ledger")+"/check", http.StatusFound)
	})
	return mux
}

type pageData struct {
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
	// Charts (uPlot): ChartJSON is safe JSON embedded in the page.
	ChartID    string
	ChartTitle string
	ChartJSON  template.JS
	NeedCharts bool
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
	p, _, err := s.loadProject()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := pageData{
		Title:       "Ledgers",
		Page:        "home",
		Ledgers:     engine.LedgerNames(p),
		ProjectRoot: p.Root,
	}
	s.render(w, "index.html", data)
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
				return out, fmt.Errorf("invalid as-of date (want YYYY-MM-DD): %s", asOfStr)
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
	page := r.PathValue("page")
	proj, pdb, err := s.loadProject()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	l, err := engine.OpenLedger(proj, pdb, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	tq, err := parsePageTimeQuery(q, time.Now(), true, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	timeStr, pr, perr, asOf, asOfStr := tq.Time, tq.Period, tq.PeriodErr, tq.AsOf, tq.AsOfStr

	data := pageData{
		Title:       name + " · " + page,
		Page:        page,
		LedgerName:  name,
		Ledgers:     engine.LedgerNames(proj),
		ProjectRoot: proj.Root,
		OpCurrency:  l.OpCurrency,
		Diags:       l.Diags,
		HasErrors:   l.Diags.HasErrors(),
		HasWarnings: l.Diags.HasWarnings(),
		OK:          !l.Diags.HasErrors(),
		AsOf:        asOfStr,
		Time:        timeStr,
		PeriodLabel: tq.PeriodLabel,
	}
	if perr != nil {
		data.Error = perr.Error()
	}

	switch page {
	case "check":
		// ok
	case "balances":
		data.BalanceRows = buildBalanceTreeRows(l.BalancesTree(asOf))
	case "journal":
		if perr == nil {
			data.Journal = l.Journal(pr.Start, pr.End)
		}
	case "pnl":
		if perr == nil {
			inc, exp := l.PnLTree(pr.Start, pr.End)
			data.IncomeRows = buildPnLRows(inc)
			data.ExpenseRows = buildPnLRows(exp)
			from, to := pr.Start, pr.End
			kind := period.ChartBin(timeStr, pr)
			bars := l.PnLBars(from, to, kind)
			if js, err := chartBarsJSON(bars, l.OpCurrency); err == nil && js != "" {
				data.NeedCharts = true
				data.ChartID = "chart-pnl"
				data.ChartTitle = "Income vs expenses"
				data.ChartJSON = js
			}
		}
	case "networth":
		tree, total, err := l.NetWorthTree(asOf)
		if err != nil {
			data.Error = err.Error()
		} else {
			data.NetWorthTot = total.FloatString(2)
			data.NetWorthRows = buildNetWorthRows(tree)
			// Event series over filter range (or full history if open).
			pts, serr := l.NetWorthSeries(pr.Start, pr.End)
			if serr == nil {
				if js, jerr := chartLineJSON(pts, l.OpCurrency, "Net worth"); jerr == nil && js != "" {
					data.NeedCharts = true
					data.ChartID = "chart-networth"
					data.ChartTitle = "Net worth over time"
					data.ChartJSON = js
				}
			}
		}
	case "documents":
		data.Documents = documentTreeRows(l.Documents)
	case "prices":
		data.PriceSeries = priceSeriesRows(pdb)
		// Chart the busiest base (or ?base=) so the prices report is not table-only.
		base := q.Get("base")
		if base == "" {
			var bestN int
			for _, row := range data.PriceSeries {
				if row.Count > bestN {
					bestN = row.Count
					base = row.Base
				}
			}
		}
		if base != "" {
			if js, quote, jerr := chartPriceJSON(pdb, base, l.OpCurrency, time.Time{}, time.Time{}); jerr == nil && js != "" {
				data.NeedCharts = true
				data.ChartID = "chart-prices"
				if quote != "" {
					data.ChartTitle = base + " price (" + quote + ")"
				} else {
					data.ChartTitle = base + " price"
				}
				data.ChartJSON = js
			}
		}
	default:
		http.NotFound(w, r)
		return
	}
	s.render(w, "ledger.html", data)
}

func (s *Server) handleAccount(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("ledger")
	account, err := url.PathUnescape(r.PathValue("account"))
	if err != nil {
		http.Error(w, "invalid account path encoding", http.StatusBadRequest)
		return
	}
	// Wildcard may use slashes if any; join
	if account == "" {
		http.NotFound(w, r)
		return
	}
	proj, pdb, err := s.loadProject()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	l, err := engine.OpenLedger(proj, pdb, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Same time query surface as ledger: ?time=, from/to, and as-of.
	tq, err := parsePageTimeQuery(r.URL.Query(), time.Now(), true, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	timeStr, pr, perr, asOf := tq.Time, tq.Period, tq.PeriodErr, tq.AsOf

	data := pageData{
		Title:       account,
		Page:        "account",
		LedgerName:  name,
		Ledgers:     engine.LedgerNames(proj),
		ProjectRoot: proj.Root,
		OpCurrency:  l.OpCurrency,
		Time:        timeStr,
		PeriodLabel: tq.PeriodLabel,
		AsOf:        tq.AsOfStr,
		AccountName: account,
	}
	if perr != nil {
		data.Error = perr.Error()
		s.render(w, "account.html", data)
		return
	}

	// balances for this account
	bals := l.AccountBalances(account, asOf)
	var brow []balanceRow
	var comms []string
	for c := range bals {
		comms = append(comms, c)
	}
	sort.Strings(comms)
	for _, c := range comms {
		brow = append(brow, balanceRow{Account: account, Commodity: c, Amount: bals[c].FloatString(4)})
	}
	data.AccountBalances = brow

	// activity in period
	act := l.AccountActivity(account, pr.Start, pr.End)
	comms = nil
	for c := range act {
		comms = append(comms, c)
	}
	sort.Strings(comms)
	var arow []balanceRow
	for _, c := range comms {
		arow = append(arow, balanceRow{Account: account, Commodity: c, Amount: act[c].FloatString(4)})
	}
	data.AccountActivity = arow

	data.Journal = l.JournalForAccount(account, pr.Start, pr.End)
	data.AccountDocs = documentRows(l.DocumentsForAccount(account))
	if info, ok := l.Accounts[account]; ok {
		data.AccountMeta = metaRows(info.Metadata)
		data.AccountCurrencies = info.Currencies
	}
	if pts, err := l.AccountSeries(account, pr.Start, pr.End); err == nil {
		if js, jerr := chartLineJSON(pts, l.OpCurrency, "Balance"); jerr == nil && js != "" {
			data.NeedCharts = true
			data.ChartID = "chart-account"
			data.ChartTitle = "Balance over time"
			data.ChartJSON = js
		}
	}
	s.render(w, "account.html", data)
}

// chartLineJSON builds uPlot line payload (event series, op currency).
func chartLineJSON(pts []engine.SeriesPoint, currency, label string) (template.JS, error) {
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
	return template.JS(b), nil
}

// chartPriceJSON builds a stepped line of market prices for base commodity.
// Prefers preferQuote (usually op currency); otherwise first series by quote name.
// Points outside [from,to] are dropped when bounds are set; zero bounds = full series.
// If the filter removes every point, falls back to the full series so the chart still shows.
func chartPriceJSON(db *prices.DB, base, preferQuote string, from, to time.Time) (template.JS, string, error) {
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
	return template.JS(b), chosen.Quote, nil
}

// chartBarsJSON builds uPlot diverging bar payload.
// X is ordinal (0..n-1), not unix time — avoids time-scale bar width/overlap artifacts.
func chartBarsJSON(bars []engine.BarPoint, currency string) (template.JS, error) {
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
	return template.JS(raw), nil
}

func ratFloat(r *big.Rat) float64 {
	if r == nil {
		return 0
	}
	f, _ := r.Float64()
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
		sort.SliceStable(byAcct[a], func(i, j int) bool {
			if byAcct[a][i].date != byAcct[a][j].date {
				return byAcct[a][i].date < byAcct[a][j].date
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
	proj, pdb, err := s.loadProject()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	l, err := engine.OpenLedger(proj, pdb, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Same time query surface as ledger: ?time=, from/to, and as-of.
	tq, err := parsePageTimeQuery(r.URL.Query(), time.Now(), true, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	timeStr, pr, perr, asOf := tq.Time, tq.Period, tq.PeriodErr, tq.AsOf

	data := pageData{
		Title:         commodity,
		Page:          "commodity",
		LedgerName:    name,
		Ledgers:       engine.LedgerNames(proj),
		ProjectRoot:   proj.Root,
		OpCurrency:    l.OpCurrency,
		Time:          timeStr,
		PeriodLabel:   tq.PeriodLabel,
		AsOf:          tq.AsOfStr,
		CommodityName: commodity,
	}
	if perr != nil {
		data.Error = perr.Error()
		s.render(w, "commodity.html", data)
		return
	}

	bals := l.CommodityBalances(commodity, asOf)
	var accts []string
	for a := range bals {
		accts = append(accts, a)
	}
	sort.Strings(accts)
	for _, a := range accts {
		data.CommodityBalances = append(data.CommodityBalances, balanceRow{
			Account: a, Commodity: commodity, Amount: bals[a].FloatString(4),
		})
	}

	act := l.CommodityActivity(commodity, pr.Start, pr.End)
	accts = nil
	for a := range act {
		accts = append(accts, a)
	}
	sort.Strings(accts)
	for _, a := range accts {
		data.CommodityActivity = append(data.CommodityActivity, balanceRow{
			Account: a, Commodity: commodity, Amount: act[a].FloatString(4),
		})
	}

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
	if js, quote, jerr := chartPriceJSON(pdb, commodity, l.OpCurrency, time.Time{}, time.Time{}); jerr == nil && js != "" {
		data.NeedCharts = true
		data.ChartID = "chart-commodity-price"
		if quote != "" {
			data.ChartTitle = "Price (" + quote + ")"
		} else {
			data.ChartTitle = "Price"
		}
		data.ChartJSON = js
	}
	s.render(w, "commodity.html", data)
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

func (s *Server) render(w http.ResponseWriter, name string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := s.Tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	if name != "" {
		return name
	}
	return account
}

func treePath(path, account string) string {
	if path != "" {
		return path
	}
	return account
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
