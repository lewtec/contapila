// Package accountslist is the web/accounts report module.
package accountslist

import (
	"sort"

	"github.com/a-h/templ"

	"github.com/lucasew/contapila-go/internal/plugin"
	"github.com/lucasew/contapila-go/internal/web"
)

// PluginKey is the journal plugin / CUE plugins map key.
const PluginKey = "web/accounts"

// PageID is the URL slug under /l/{ledger}/{PageID}.
const PageID = "accounts"

// Options is plugins."web/accounts".
type Options struct {
	Enabled bool `json:"enabled"`
}

func init() {
	plugin.RegisterTyped(plugin.TypedModule[Options]{
		ID: PluginKey,
		AttachWeb: func() {
			web.ContributePage(web.Page{
				ID:      PageID,
				Label:   "Accounts",
				Order:   25,
				Sidebar: true,
				Build:   true,
				Fill:    fill,
				Body:    func(d web.PageData) templ.Component { return accountsBody(d) },
			})
		},
	})
}

func fill(pc web.PageContext, data *web.PageData) {
	if pc.Ledger == nil {
		return
	}
	names := make([]string, 0, len(pc.Ledger.Accounts))
	for name := range pc.Ledger.Accounts {
		names = append(names, name)
	}
	sort.Strings(names)
	rows := make([]web.AccountListRow, 0, len(names))
	for _, name := range names {
		info := pc.Ledger.Accounts[name]
		open := ""
		if !info.OpenDate.IsZero() {
			open = info.OpenDate.Format("2006-01-02")
		}
		curs := append([]string(nil), info.Currencies...)
		rows = append(rows, web.AccountListRow{
			Account:    name,
			OpenDate:   open,
			Currencies: curs,
		})
	}
	data.AccountList = rows
}
