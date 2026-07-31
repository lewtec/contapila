package web

import (
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/a-h/templ"

	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/period"
	"github.com/lucasew/contapila-go/internal/prices"
	"github.com/lucasew/contapila-go/pkg/project"
)

// Page is one ledger report under GET /l/{ledger}/{ID}.
// Registering a page wires sidebar (optional), static build expansion (optional),
// request fill, and the templ body — one table for routes + nav + build.
type Page struct {
	// ID is the URL slug (e.g. "check", "pnl"). Required and unique.
	ID string
	// Label is the sidebar text and breadcrumb title.
	Label string
	// Order sorts sidebar entries (lower first). Build expansion follows registration order
	// of pages with Build set, not Order.
	Order int
	// Sidebar includes this page in the ledger reports rail.
	Sidebar bool
	// Build materializes /l/{ledger}/{ID} during contapila build.
	Build bool
	// DefaultEnabled: when true, the page is on if contapila.cue omits the plugin key.
	// When false, omit means off (opt-in). Explicit plugins.<key>.enabled always wins.
	// Only applies to plugin pages; core builtins ignore this.
	DefaultEnabled bool
	// PluginKey is the plugins map key that enables this page (set by plugin Web middleman).
	// Empty means PagePluginKey(ID) (web/{ID}).
	PluginKey string
	// Fill loads report data into PageData after base shell fields and diags are set.
	// Nil means no extra data (e.g. check uses only diags).
	Fill func(pc PageContext, data *PageData)
	// Body returns the main content inside reportLayout. Required for a usable page.
	Body func(d PageData) templ.Component
}

// PageContext is the live request + ledger snapshot passed to Page.Fill.
type PageContext struct {
	Request   *http.Request
	Project   *project.Project
	Prices    *prices.DB
	Ledger    *engine.Ledger
	TimeQuery pageTimeQuery
	Period    period.Range
	PeriodErr error
	AsOf      time.Time
}

// PageRegistry is the table of ledger report pages (sidebar + routes + build).
type PageRegistry struct {
	pages []Page
	byID  map[string]int // id → index in pages
}

// NewPageRegistry returns an empty registry.
func NewPageRegistry() *PageRegistry {
	return &PageRegistry{byID: map[string]int{}}
}

// Register adds a ledger page. Panics on empty ID, missing Body, or duplicate ID.
func (r *PageRegistry) Register(p Page) {
	if r == nil {
		panic("web: Register on nil PageRegistry")
	}
	if p.ID == "" {
		panic("web: page with empty ID")
	}
	if p.Body == nil {
		panic("web: page " + p.ID + " missing Body")
	}
	if r.byID == nil {
		r.byID = map[string]int{}
	}
	if _, dup := r.byID[p.ID]; dup {
		panic("web: duplicate page ID " + p.ID)
	}
	r.byID[p.ID] = len(r.pages)
	r.pages = append(r.pages, p)
}

// Lookup returns the page for id.
func (r *PageRegistry) Lookup(id string) (Page, bool) {
	if r == nil || r.byID == nil {
		return Page{}, false
	}
	i, ok := r.byID[id]
	if !ok {
		return Page{}, false
	}
	return r.pages[i], true
}

// Sidebar returns pages with Sidebar set, sorted by Order then ID.
func (r *PageRegistry) Sidebar() []Page {
	if r == nil {
		return nil
	}
	var out []Page
	for _, p := range r.pages {
		if p.Sidebar {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// BuildIDs returns IDs of pages with Build set, in registration order.
func (r *PageRegistry) BuildIDs() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.pages))
	for _, p := range r.pages {
		if p.Build {
			out = append(out, p.ID)
		}
	}
	return out
}

// ReportPages is the composed ledger report slug list (builtins + plugin pages),
// without CUE plugin filtering. Used by tests and as the pre-filter expand set.
func ReportPages() []string {
	return ComposePages(DefaultPages(), pluginContributedPages()...).BuildIDs()
}

// DefaultPages returns the built-in ledger report registry (singleton).
// Callers that add pages must Clone or ComposePages — do not Register on this value.
func DefaultPages() *PageRegistry {
	return defaultPages()
}

var defaultPages = sync.OnceValue(func() *PageRegistry {
	r := NewPageRegistry()
	registerBuiltinPages(r)
	return r
})

// resolvedPages is the live registry: explicit s.Pages, else core builtins plus
// plugin pages enabled in contapila.cue (opt-in; core reports always on).
func (s *Server) resolvedPages(sess *Session) *PageRegistry {
	if s != nil && s.Pages != nil {
		return s.Pages
	}
	builtins := DefaultPages().Clone()
	contrib := pluginContributedPages()
	if len(contrib) == 0 {
		return builtins
	}
	if sess == nil {
		// No project yet: include all contributed (CLI/tests without session).
		return ComposePages(builtins, contrib...)
	}
	p, _, err := sess.Project()
	if err != nil || p == nil || p.Config == nil {
		return ComposePages(builtins, contrib...)
	}
	flags, err := config.PluginFlags(p.Config.Value)
	if err != nil {
		// Opt-in: on decode failure, ship core only (no blanking builtins).
		return builtins
	}
	return ComposePages(builtins, filterContribPages(contrib, flags)...)
}

// filterContribPages keeps plugin pages enabled by CUE or DefaultEnabled.
// Explicit plugins.<PluginKey>.enabled wins; missing key uses Page.DefaultEnabled.
func filterContribPages(pages []Page, flags map[string]bool) []Page {
	if len(pages) == 0 {
		return nil
	}
	var out []Page
	for _, p := range pages {
		key := p.PluginKey
		if key == "" {
			key = PagePluginKey(p.ID)
		}
		if flags != nil {
			if b, ok := flags[key]; ok {
				if b {
					out = append(out, p)
				}
				continue
			}
		}
		if p.DefaultEnabled {
			out = append(out, p)
		}
	}
	return out
}

// pageRegistry is resolvedPages without a session (no CUE filter).
func (s *Server) pageRegistry() *PageRegistry {
	return s.resolvedPages(nil)
}

// pagesFor resolves the registry on page data (custom or default builtins).
func pagesFor(d PageData) *PageRegistry {
	if d.Pages != nil {
		return d.Pages
	}
	return DefaultPages()
}

// sidebarReportPages is used by layout.templ for the Reports rail.
func sidebarReportPages(d PageData) []Page {
	return pagesFor(d).Sidebar()
}

// pageBody resolves the registered Body for d.Page (empty component if unknown).
func pageBody(d PageData) templ.Component {
	if p, ok := pagesFor(d).Lookup(d.Page); ok && p.Body != nil {
		return p.Body(d)
	}
	// Unknown pages should not reach render (handler 404s first).
	return templ.NopComponent
}

// pageLabel returns the sidebar/breadcrumb label for a report id, or id itself.
func pageLabel(d PageData) string {
	if p, ok := pagesFor(d).Lookup(d.Page); ok && p.Label != "" {
		return p.Label
	}
	switch d.Page {
	case "account":
		return d.AccountName
	case "commodity":
		return d.CommodityName
	case "":
		return "Ledgers"
	default:
		return d.Page
	}
}

func registerBuiltinPages(r *PageRegistry) {
	r.Register(Page{
		ID: "check", Label: "Check", Order: 10, Sidebar: true, Build: true,
		Body: func(d PageData) templ.Component { return checkBody(d) },
	})
	r.Register(Page{
		ID: "balances", Label: "Balances", Order: 20, Sidebar: true, Build: true,
		Fill: fillBalances,
		Body: func(d PageData) templ.Component { return balancesBody(d) },
	})
	r.Register(Page{
		ID: "journal", Label: "Journal", Order: 30, Sidebar: true, Build: true,
		Fill: fillJournal,
		Body: func(d PageData) templ.Component { return journalList(d) },
	})
	r.Register(Page{
		ID: "pnl", Label: "Income statement", Order: 40, Sidebar: true, Build: true,
		Fill: fillPnL,
		Body: func(d PageData) templ.Component { return pnlBody(d) },
	})
	r.Register(Page{
		ID: "networth", Label: "Net worth", Order: 50, Sidebar: true, Build: true,
		Fill: fillNetWorth,
		Body: func(d PageData) templ.Component { return networthBody(d) },
	})
	r.Register(Page{
		ID: "documents", Label: "Documents", Order: 60, Sidebar: true, Build: true,
		Fill: fillDocuments,
		Body: func(d PageData) templ.Component { return documentsBody(d) },
	})
	r.Register(Page{
		ID: "prices", Label: "Prices", Order: 70, Sidebar: true, Build: true,
		Fill: fillPrices,
		Body: func(d PageData) templ.Component { return pricesBody(d) },
	})
	r.Register(Page{
		ID: "debug", Label: "Debug", Order: 100, Sidebar: true, Build: true,
		Fill: fillDebug,
		Body: func(d PageData) templ.Component { return debugBody(d) },
	})
}

func fillBalances(pc PageContext, data *PageData) {
	data.BalanceRows = buildBalanceTreeRows(pc.Ledger.BalancesTree(pc.AsOf))
}

func fillJournal(pc PageContext, data *PageData) {
	if pc.PeriodErr == nil {
		data.Journal = pc.Ledger.Journal(pc.Period.Start, pc.Period.End)
	}
}

func fillPnL(pc PageContext, data *PageData) {
	if pc.PeriodErr != nil {
		return
	}
	l := pc.Ledger
	inc, exp := l.PnLTree(pc.Period.Start, pc.Period.End)
	data.IncomeRows = buildPnLRows(inc)
	data.ExpenseRows = buildPnLRows(exp)
	kind := period.ChartBin(pc.TimeQuery.Time, pc.Period)
	bars := l.PnLBars(pc.Period.Start, pc.Period.End, kind)
	if js, err := chartBarsJSON(bars, l.OpCurrency); err == nil {
		setChart(data, "chart-pnl", "Income vs expenses", js)
	}
}

func fillNetWorth(pc PageContext, data *PageData) {
	l := pc.Ledger
	tree, total, err := l.NetWorthTree(pc.AsOf)
	if err != nil {
		data.Error = err.Error()
		return
	}
	data.NetWorthTot = total.FloatString(2)
	data.NetWorthRows = buildNetWorthRows(tree)
	if pts, serr := l.NetWorthSeries(pc.Period.Start, pc.Period.End); serr == nil {
		if js, jerr := chartLineJSON(pts, l.OpCurrency, "Net worth"); jerr == nil {
			setChart(data, "chart-networth", "Net worth over time", js)
		}
	}
}

func fillDocuments(pc PageContext, data *PageData) {
	data.Documents = documentTreeRows(pc.Ledger.Documents)
}

func fillPrices(pc PageContext, data *PageData) {
	data.PriceSeries = priceSeriesRows(pc.Prices)
	base := ""
	if pc.Request != nil {
		base = pc.Request.URL.Query().Get("base")
	}
	if base == "" {
		var bestN int
		for _, row := range data.PriceSeries {
			if row.Count > bestN {
				bestN = row.Count
				base = row.Base
			}
		}
	}
	if base == "" {
		return
	}
	l := pc.Ledger
	if js, quote, jerr := chartPriceJSON(pc.Prices, base, l.OpCurrency, time.Time{}, time.Time{}); jerr == nil {
		title := base + " price"
		if quote != "" {
			title = base + " price (" + quote + ")"
		}
		setChart(data, "chart-prices", title, js)
	}
}
