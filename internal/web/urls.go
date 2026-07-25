package web

import (
	"net/url"
	"strconv"
)

func ledgerURL(ledger, page, timeFilter string) string {
	u := "/l/" + url.PathEscape(ledger) + "/" + url.PathEscape(page)
	if timeFilter != "" {
		u += "?time=" + url.QueryEscape(timeFilter)
	}
	return u
}

func accountURL(ledger, account, timeFilter string) string {
	// Keep ":" readable in URLs; PathEscape only encodes reserved bits.
	u := "/l/" + url.PathEscape(ledger) + "/account/" + url.PathEscape(account)
	if timeFilter != "" {
		u += "?time=" + url.QueryEscape(timeFilter)
	}
	return u
}

func commodityURL(ledger, commodity, timeFilter string) string {
	u := "/l/" + url.PathEscape(ledger) + "/commodity/" + url.PathEscape(commodity)
	if timeFilter != "" {
		u += "?time=" + url.QueryEscape(timeFilter)
	}
	return u
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
	switch d.Page {
	case "account":
		return accountURL(d.LedgerName, d.AccountName, "")
	case "commodity":
		return commodityURL(d.LedgerName, d.CommodityName, "")
	default:
		return "/l/" + url.PathEscape(d.LedgerName) + "/" + url.PathEscape(d.Page)
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
