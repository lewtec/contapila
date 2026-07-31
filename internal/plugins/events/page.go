// Package events is the web_events report module (journal event directives).
package events

import (
	"sort"

	"github.com/a-h/templ"

	"github.com/lucasew/contapila-go/internal/plugin"
	"github.com/lucasew/contapila-go/internal/web"
)

// PluginKey is the journal plugin / CUE plugins map key (no '/').
const PluginKey = "web_events"

// PageID is the URL slug under /l/{ledger}/{PageID}.
const PageID = "events"

// Settings is plugins.web_events.settings (none yet).
type Settings struct{}

func init() {
	plugin.RegisterTyped(PluginKey, func(reg *plugin.Reg[Settings]) {
		reg.Page(web.Page{
			ID:      PageID,
			Label:   "Events",
			Order:   55,
			Sidebar: true,
			Build:   true,
			Fill:    fill,
			Body:    func(d web.PageData) templ.Component { return eventsBody(d) },
		})
	})
}

func fill(pc web.PageContext, data *web.PageData) {
	if pc.Ledger == nil || pc.Ledger.Book == nil {
		return
	}
	evs := pc.Ledger.Book.Events
	rows := make([]web.EventListRow, 0, len(evs))
	for _, e := range evs {
		date := ""
		if !e.Date.IsZero() {
			date = e.Date.Format("2006-01-02")
		}
		rows = append(rows, web.EventListRow{
			Date: date,
			Type: e.Type,
			Desc: e.Desc,
		})
	}
	// Newest first (same convention as journal listings).
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date > rows[j].Date
		}
		return rows[i].Type < rows[j].Type
	})
	data.EventList = rows
}
