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

// PathLedgerRoot is GET /l/{ledger}/ (live redirect to check).
func PathLedgerRoot(ledger string) string {
	return "/l/" + url.PathEscape(ledger) + "/"
}

// Session link methods honor Static for on-disk hrefs in contapila build.

func (s *Session) staticMode() bool { return s != nil && s.Static }

// HomeURL is the site root (breadcrumb "contapila").
func (s *Session) HomeURL() string {
	if s.staticMode() {
		return "/index.html"
	}
	return "/"
}

// LedgerURL is a report page link (check, pnl, …).
func (s *Session) LedgerURL(ledger, page, timeFilter string) string {
	u := PathLedger(ledger, page)
	if s.staticMode() {
		return u + ".html"
	}
	if timeFilter != "" {
		u += "?time=" + url.QueryEscape(timeFilter)
	}
	return u
}

// AccountURL is an account page link.
func (s *Session) AccountURL(ledger, account, timeFilter string) string {
	u := PathAccount(ledger, account)
	if s.staticMode() {
		return u + ".html"
	}
	if timeFilter != "" {
		u += "?time=" + url.QueryEscape(timeFilter)
	}
	return u
}

// CommodityURL is a commodity page link.
func (s *Session) CommodityURL(ledger, commodity, timeFilter string) string {
	u := PathCommodity(ledger, commodity)
	if s.staticMode() {
		return u + ".html"
	}
	if timeFilter != "" {
		u += "?time=" + url.QueryEscape(timeFilter)
	}
	return u
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

func homeURL(s *Session) string {
	if s == nil {
		return (&Session{}).HomeURL()
	}
	return s.HomeURL()
}

func ledgerURL(s *Session, ledger, page, timeFilter string) string {
	if s == nil {
		return (&Session{}).LedgerURL(ledger, page, timeFilter)
	}
	return s.LedgerURL(ledger, page, timeFilter)
}

func accountURL(s *Session, ledger, account, timeFilter string) string {
	if s == nil {
		return (&Session{}).AccountURL(ledger, account, timeFilter)
	}
	return s.AccountURL(ledger, account, timeFilter)
}

func commodityURL(s *Session, ledger, commodity, timeFilter string) string {
	if s == nil {
		return (&Session{}).CommodityURL(ledger, commodity, timeFilter)
	}
	return s.CommodityURL(ledger, commodity, timeFilter)
}

func pageTitle(d pageData) string {
	switch d.Page {
	case "pnl":
		return "Income statement"
	case "networth":
		return "Net worth"
	case "check":
		return "Check"
	case "balances":
		return "Balances"
	case "journal":
		return "Journal"
	case "documents":
		return "Documents"
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

func timeFilterAction(d pageData) string {
	s := d.Sess
	switch d.Page {
	case "account":
		return accountURL(s, d.LedgerName, d.AccountName, "")
	case "commodity":
		return commodityURL(s, d.LedgerName, d.CommodityName, "")
	default:
		return ledgerURL(s, d.LedgerName, d.Page, "")
	}
}

func chartAriaLabel(d pageData) string {
	if d.ChartTitle != "" {
		return d.ChartTitle
	}
	return "Chart"
}

func treeDataAccount(path, account string) string {
	return cmp.Or(path, account)
}

func fmtInt(n int) string {
	return strconv.Itoa(n)
}
