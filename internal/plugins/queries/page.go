// Package queries is the web_queries report module (journal query directives).
package queries

import (
	"github.com/a-h/templ"

	"github.com/lucasew/contapila-go/internal/plugin"
	"github.com/lucasew/contapila-go/internal/web"
)

// PluginKey is the journal plugin / CUE plugins map key (no '/').
const PluginKey = "web_queries"

// PageID is the URL slug under /l/{ledger}/{PageID}.
const PageID = "queries"

// Settings is plugins.web_queries.settings (none yet).
type Settings struct{}

func init() {
	plugin.RegisterTyped(PluginKey, func(reg *plugin.Reg[Settings]) {
		reg.Page(web.Page{
			ID:             PageID,
			Label:          "Queries",
			Order:          65,
			Sidebar:        false, // each named query is listed under sidebar Queries
			Build:          true,
			DefaultEnabled: true,
			Fill:           fill,
			Body:           func(d web.PageData) templ.Component { return queriesBody(d) },
		})
	})
}

func fill(pc web.PageContext, data *web.PageData) {
	if pc.Ledger == nil || pc.Ledger.Book == nil {
		return
	}
	qs := pc.Ledger.Book.Queries
	rows := make([]web.QueryListRow, 0, len(qs))
	for _, q := range qs {
		date := ""
		if !q.Date.IsZero() {
			date = q.Date.Format("2006-01-02")
		}
		rows = append(rows, web.QueryListRow{
			Date: date,
			Name: q.Name,
			Text: q.Query,
		})
	}
	data.QueryList = rows
}
