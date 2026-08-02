package web

import (
	"log/slog"
	"math/big"
	"net/http"
	"sort"
	"time"

	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/prices"
	"github.com/lucasew/contapila-go/pkg/project"
)

// openLedgerRequest resolves project + ledger via the request Session (middleware
// or build-attached), then parses the shared time/as-of query.
// On failure it writes the HTTP error and returns ok=false.
func (s *Server) openLedgerRequest(w http.ResponseWriter, r *http.Request, ledgerName string) (
	proj *project.Project, pdb *prices.DB, l *engine.Ledger, tq pageTimeQuery, sess *Session, ok bool,
) {
	sess = sessionFrom(r.Context())
	if sess == nil {
		sess = NewSession(s.Root)
	}
	var err error
	proj, pdb, err = sess.Project()
	if err != nil {
		slog.Error("web load project", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	l, err = sess.Ledger(ledgerName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tq, err = parsePageTimeQuery(r.URL.Query(), time.Now(), true, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ok = true
	return
}

// basePageData fills ledger-scoped shell fields shared by report/account/commodity pages.
// Pages is left nil (DefaultPages via pagesFor) unless the caller sets resolvedPages.
func basePageData(sess *Session, proj *project.Project, l *engine.Ledger, ledgerName, title, page string, tq pageTimeQuery) pageData {
	data := pageData{
		Sess:        sess,
		Title:       title,
		Page:        page,
		LedgerName:  ledgerName,
		Ledgers:     engine.LedgerNames(proj),
		ProjectRoot: proj.Root,
		OpCurrency:  l.OpCurrency,
		Time:        tq.Time,
		PeriodLabel: tq.PeriodLabel,
		AsOf:        tq.AsOfStr,
	}
	if tq.PeriodErr != nil {
		data.Error = tq.PeriodErr.Error()
	}
	data.QueryNav = queryNavItems(l)
	data.JumpAccounts = jumpAccountNames(l)
	data.JumpCommodities = jumpCommodityNames(l)
	return data
}

// queryNavItems builds the sidebar query list from booked journal queries (name order).
func queryNavItems(l *engine.Ledger) []QueryNavItem {
	if l == nil || l.Book == nil || len(l.Book.Queries) == 0 {
		return nil
	}
	// Preserve journal order; drop empty names; de-dupe by first occurrence.
	seen := map[string]struct{}{}
	out := make([]QueryNavItem, 0, len(l.Book.Queries))
	for _, q := range l.Book.Queries {
		name := q.Name
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, QueryNavItem{Name: name})
	}
	return out
}

// queriesUIEnabled is true when the web_queries page is in the resolved registry.
func queriesUIEnabled(d PageData) bool {
	_, ok := pagesFor(d).Lookup("queries")
	return ok
}

// basePageData attaches this server's resolved PageRegistry (sidebar / body lookup).
func (s *Server) basePageData(sess *Session, proj *project.Project, l *engine.Ledger, ledgerName, title, page string, tq pageTimeQuery) pageData {
	data := basePageData(sess, proj, l, ledgerName, title, page, tq)
	data.Pages = s.resolvedPages(sess)
	return data
}

// setChart attaches a non-empty chart payload to the page.
func setChart(d *PageData, id, title, js string) {
	if js == "" {
		return
	}
	d.NeedCharts = true
	d.ChartID = id
	d.ChartTitle = title
	d.ChartJSON = js
}

// amountMapRows turns map[key]*big.Rat into sorted balance rows.
// When fixedAccount is set, map keys are commodities; otherwise keys are accounts
// and fixedCommodity is applied to every row.
func amountMapRows(m map[string]*big.Rat, fixedAccount, fixedCommodity string, prec int) []balanceRow {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]balanceRow, 0, len(keys))
	for _, k := range keys {
		n := m[k]
		amt := ""
		if n != nil {
			amt = n.FloatString(prec)
		}
		row := balanceRow{Amount: amt}
		if fixedAccount != "" {
			row.Account = fixedAccount
			row.Commodity = k
		} else {
			row.Account = k
			row.Commodity = fixedCommodity
		}
		out = append(out, row)
	}
	return out
}
