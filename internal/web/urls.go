package web

import (
	"cmp"
	"net/url"
	"strconv"
)

// HTTP route paths (mux / expand / httptest). Never include .html — handlers
// serve extensionless live paths; static files are a build-time fileRel mapping.

// PathLedger is GET /l/{ledger}/{page}.
func PathLedger(ledger, page string) string {
	return "/l/" + url.PathEscape(ledger) + "/" + url.PathEscape(page)
}

// PathAccount is GET /l/{ledger}/account/{account}.
func PathAccount(ledger, account string) string {
	return "/l/" + url.PathEscape(ledger) + "/account/" + url.PathEscape(account)
}

// PathCommodity is GET /l/{ledger}/commodity/{commodity}.
func PathCommodity(ledger, commodity string) string {
	return "/l/" + url.PathEscape(ledger) + "/commodity/" + url.PathEscape(commodity)
}

// PathQuery is GET /l/{ledger}/query/{name}.
func PathQuery(ledger, name string) string {
	return "/l/" + url.PathEscape(ledger) + "/query/" + url.PathEscape(name)
}

// PathLedgerRoot is GET /l/{ledger}/ (live redirect to check).
func PathLedgerRoot(ledger string) string {
	return "/l/" + url.PathEscape(ledger) + "/"
}

// Session link methods honor Static for on-disk hrefs in contapila build.

func (s *Session) staticMode() bool { return s != nil && s.Static }

// linkURL maps a live path to the static or filtered form used in hrefs.
// Static builds append .html (no query). Live keeps the path and optional ?time=.
func (s *Session) linkURL(u, timeFilter string) string {
	if s.staticMode() {
		return u + ".html"
	}
	if timeFilter != "" {
		u += "?time=" + url.QueryEscape(timeFilter)
	}
	return u
}

// HomeURL is the site root (breadcrumb "contapila").
func (s *Session) HomeURL() string {
	if s.staticMode() {
		return "/index.html"
	}
	return "/"
}

// LedgerURL is a report page link (check, pnl, …).
func (s *Session) LedgerURL(ledger, page, timeFilter string) string {
	return s.linkURL(PathLedger(ledger, page), timeFilter)
}

// AccountURL is an account page link.
func (s *Session) AccountURL(ledger, account, timeFilter string) string {
	return s.linkURL(PathAccount(ledger, account), timeFilter)
}

// CommodityURL is a commodity page link.
func (s *Session) CommodityURL(ledger, commodity, timeFilter string) string {
	return s.linkURL(PathCommodity(ledger, commodity), timeFilter)
}

// QueryURL is a named query page link.
func (s *Session) QueryURL(ledger, name, timeFilter string) string {
	return s.linkURL(PathQuery(ledger, name), timeFilter)
}

// LedgerRootURL is the ledger landing path (/l/x/ live, /l/x/index.html static).
func (s *Session) LedgerRootURL(ledger string) string {
	if s.staticMode() {
		return PathLedgerRoot(ledger) + "index.html"
	}
	return PathLedgerRoot(ledger)
}

// Package helpers for templ: nil Session ⇒ live (non-static) URLs.

// sessionStatic reports whether links/UI should use the static-site shape.
func sessionStatic(s *Session) bool {
	return s != nil && s.Static
}

// orSession returns s, or an empty Session when s is nil (live URL defaults).
func orSession(s *Session) *Session {
	if s == nil {
		return &Session{}
	}
	return s
}

func homeURL(s *Session) string {
	return orSession(s).HomeURL()
}

func ledgerURL(s *Session, ledger, page, timeFilter string) string {
	return orSession(s).LedgerURL(ledger, page, timeFilter)
}

func accountURL(s *Session, ledger, account, timeFilter string) string {
	return orSession(s).AccountURL(ledger, account, timeFilter)
}

func commodityURL(s *Session, ledger, commodity, timeFilter string) string {
	return orSession(s).CommodityURL(ledger, commodity, timeFilter)
}

func queryURL(s *Session, ledger, name, timeFilter string) string {
	return orSession(s).QueryURL(ledger, name, timeFilter)
}

func pageTitle(d PageData) string {
	// Report labels come from PageRegistry; account/commodity/home handled in pageLabel.
	return pageLabel(d)
}

func timeFilterAction(d PageData) string {
	s := d.Sess
	switch d.Page {
	case "account":
		return accountURL(s, d.LedgerName, d.AccountName, "")
	case "commodity":
		return commodityURL(s, d.LedgerName, d.CommodityName, "")
	case "query":
		return queryURL(s, d.LedgerName, d.QueryName, "")
	default:
		return ledgerURL(s, d.LedgerName, d.Page, "")
	}
}

func chartAriaLabel(d PageData) string {
	return cmp.Or(d.ChartTitle, "Chart")
}

func treeDataAccount(path, account string) string {
	return cmp.Or(path, account)
}

func fmtInt(n int) string {
	return strconv.Itoa(n)
}
