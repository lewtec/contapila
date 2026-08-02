package web

import (
	"sort"

	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/prices"
	"github.com/lucasew/contapila-go/pkg/cmdpalette"
	"github.com/lucasew/contapila-go/pkg/project"
)

// CommandItem is a palette row (alias of the reusable cmdpalette.Item).
type CommandItem = cmdpalette.Item

// Palette is a command-palette contribution registered via plugin.Reg.Palette.
// Enablement matches Page: DefaultEnabled when CUE omits plugins.<module>.enabled.
// UI lives in pkg/cmdpalette; this type is only the plugin hook.
type Palette struct {
	// DefaultEnabled: when true, on if contapila.cue omits the plugin key.
	// When false, omit means off (opt-in). Explicit plugins.<key>.enabled wins.
	DefaultEnabled bool
	// Fill returns extra palette rows for the current ledger shell. Required.
	// Use AccountHref / LedgerHref / QueryHref / … so static builds get .html paths.
	Fill func(pc PageContext) []CommandItem
}

// Command builds a CommandItem. Keywords defaults to group+label (+ extras).
func Command(group, label, href string, extraKeywords ...string) CommandItem {
	return cmdpalette.ItemOf(group, label, href, extraKeywords...)
}

// jumpPalette builds the reusable cmdpalette for this page shell.
func jumpPalette(d pageData) cmdpalette.Palette {
	return cmdpalette.New(commandItems(d)).
		WithID("cmdpalette").
		WithTitle("Jump").
		WithPlaceholder("Jump to report, account, ledger…").
		WithButtonClass("btn btn-ghost btn-xs h-6 min-h-0 gap-1 px-1.5 font-normal text-base-content/80 hover:text-base-content border border-base-300/80")
}

// commandItems builds the palette catalog for the current page shell.
// Home: ledgers only. Ledger-scoped: reports, queries, accounts, commodities,
// other ledgers, then PluginCommands (from plugin.Reg.Palette).
func commandItems(d pageData) []CommandItem {
	var out []CommandItem

	for _, name := range d.Ledgers {
		if name == "" {
			continue
		}
		out = append(out, Command(
			"Ledgers", name, ledgerSwitchHref(d, name), "ledger",
		))
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
			out = append(out, Command(
				sec.Title, label,
				ledgerURL(d.Sess, d.LedgerName, p.ID, d.Time),
				p.ID,
			))
		}
		for _, qname := range sec.Queries {
			if qname == "" {
				continue
			}
			out = append(out, Command(
				sec.Title, qname,
				queryURL(d.Sess, d.LedgerName, qname, d.Time),
				"query",
			))
		}
	}

	for _, a := range d.JumpAccounts {
		if a == "" {
			continue
		}
		out = append(out, Command(
			"Accounts", a,
			accountURL(d.Sess, d.LedgerName, a, d.Time),
			"account",
		))
	}

	for _, c := range d.JumpCommodities {
		if c == "" {
			continue
		}
		out = append(out, Command(
			"Commodities", c,
			commodityURL(d.Sess, d.LedgerName, c, d.Time),
			"commodity",
		))
	}

	for _, it := range d.PluginCommands {
		if n, ok := cmdpalette.Normalize(it); ok {
			out = append(out, n)
		}
	}

	return out
}

// collectPluginCommands runs enabled palette hooks for this ledger shell.
func collectPluginCommands(sess *Session, proj *project.Project, pdb *prices.DB, l *engine.Ledger, ledgerName string, tq pageTimeQuery) []CommandItem {
	entries := pluginContributedPalettes()
	if len(entries) == 0 {
		return nil
	}
	var flags map[string]bool
	if proj != nil && proj.Config != nil {
		if f, err := config.PluginFlags(proj.Config.Value); err == nil {
			flags = f
		}
	}
	pc := PageContext{
		Sess:       sess,
		Project:    proj,
		Prices:     pdb,
		Ledger:     l,
		LedgerName: ledgerName,
		Time:       tq.Time,
		TimeQuery:  tq,
		Period:     tq.Period,
		PeriodErr:  tq.PeriodErr,
		AsOf:       tq.AsOf,
	}
	return applyPaletteEntries(entries, flags, pc)
}

// applyPaletteEntries runs Fill for enabled entries (testable without BindWeb).
func applyPaletteEntries(entries []paletteEntry, flags map[string]bool, pc PageContext) []CommandItem {
	var out []CommandItem
	for _, e := range entries {
		if !pluginContribEnabled(e.pluginKey, e.defaultEnabled, flags) {
			continue
		}
		if e.fill == nil {
			continue
		}
		for _, it := range e.fill(pc) {
			if n, ok := cmdpalette.Normalize(it); ok {
				out = append(out, n)
			}
		}
	}
	return out
}

// pluginContribEnabled matches filterContribPages: explicit CUE wins, else default.
func pluginContribEnabled(key string, defaultEnabled bool, flags map[string]bool) bool {
	if flags != nil {
		if b, ok := flags[key]; ok {
			return b
		}
	}
	return defaultEnabled
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
