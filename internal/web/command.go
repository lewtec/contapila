package web

import (
	"sort"
	"strings"

	"github.com/lucasew/contapila-go/internal/engine"
)

// CommandItem is one row in the command palette (SSR list; client filters).
type CommandItem struct {
	Group    string // section label: Ledgers, Reports, Queries, Accounts, Commodities
	Label    string // primary text
	Href     string // navigation target (live or static-shaped)
	Keywords string // lowercase haystack for client-side filter
}

// commandItems builds the palette catalog for the current page shell.
// Home: ledgers only. Ledger-scoped: reports, queries, accounts, commodities, other ledgers.
func commandItems(d pageData) []CommandItem {
	var out []CommandItem

	for _, name := range d.Ledgers {
		if name == "" {
			continue
		}
		out = append(out, CommandItem{
			Group:    "Ledgers",
			Label:    name,
			Href:     ledgerSwitchHref(d, name),
			Keywords: keywords("ledger", name),
		})
	}

	if d.LedgerName == "" {
		return out
	}

	for _, sec := range sidebarNavSections(d) {
		for _, p := range sec.Pages {
			if p.ID == "" {
				continue
			}
			label := p.Label
			if label == "" {
				label = p.ID
			}
			out = append(out, CommandItem{
				Group:    sec.Title,
				Label:    label,
				Href:     ledgerURL(d.Sess, d.LedgerName, p.ID, d.Time),
				Keywords: keywords(sec.Title, label, p.ID),
			})
		}
		for _, qname := range sec.Queries {
			if qname == "" {
				continue
			}
			out = append(out, CommandItem{
				Group:    sec.Title,
				Label:    qname,
				Href:     queryURL(d.Sess, d.LedgerName, qname, d.Time),
				Keywords: keywords("query", qname),
			})
		}
	}

	for _, a := range d.JumpAccounts {
		if a == "" {
			continue
		}
		out = append(out, CommandItem{
			Group:    "Accounts",
			Label:    a,
			Href:     accountURL(d.Sess, d.LedgerName, a, d.Time),
			Keywords: keywords("account", a),
		})
	}

	for _, c := range d.JumpCommodities {
		if c == "" {
			continue
		}
		out = append(out, CommandItem{
			Group:    "Commodities",
			Label:    c,
			Href:     commodityURL(d.Sess, d.LedgerName, c, d.Time),
			Keywords: keywords("commodity", c),
		})
	}

	return out
}

// ledgerSwitchHref picks a destination when jumping ledgers.
// Preserve report page + time when possible; entity pages fall back to check.
func ledgerSwitchHref(d pageData, ledger string) string {
	page := d.Page
	switch page {
	case "", "home", "account", "commodity", "query":
		page = "check"
	}
	return ledgerURL(d.Sess, ledger, page, d.Time)
}

func keywords(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strings.ToLower(p))
	}
	s := b.String()
	// Index colon-separated account segments for substring match.
	if strings.Contains(s, ":") {
		s = s + " " + strings.ReplaceAll(s, ":", " ")
	}
	return s
}

// jumpAccountNames returns sorted account keys for the palette.
func jumpAccountNames(l *engine.Ledger) []string {
	if l == nil || len(l.Accounts) == 0 {
		return nil
	}
	out := make([]string, 0, len(l.Accounts))
	for name := range l.Accounts {
		if name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// jumpCommodityNames returns sorted commodity keys for the palette.
func jumpCommodityNames(l *engine.Ledger) []string {
	if l == nil || len(l.Commodities) == 0 {
		return nil
	}
	out := make([]string, 0, len(l.Commodities))
	for name := range l.Commodities {
		if name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
