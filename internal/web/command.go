package web

import (
	"sort"
	"strings"

	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/prices"
	"github.com/lucasew/contapila-go/pkg/project"
)

// CommandItem is one row in the command palette (SSR list; client filters).
type CommandItem struct {
	Group    string // section label: Ledgers, Reports, Queries, Accounts, Commodities, …
	Label    string // primary text
	Href     string // navigation target (live or static-shaped)
	Keywords string // lowercase haystack for client-side filter
}

// Palette is a command-palette contribution registered via plugin.Reg.Palette.
// Enablement matches Page: DefaultEnabled when CUE omits plugins.<module>.enabled.
type Palette struct {
	// DefaultEnabled: when true, on if contapila.cue omits the plugin key.
	// When false, omit means off (opt-in). Explicit plugins.<key>.enabled wins.
	DefaultEnabled bool
	// Fill returns extra palette rows for the current ledger shell. Required.
	// Called on every ledger-scoped page render when the module is enabled.
	// Use AccountHref / LedgerHref / QueryHref / … so static builds get .html paths.
	Fill func(pc PageContext) []CommandItem
}

// Command builds a CommandItem. Keywords defaults to group+label (+ extras).
func Command(group, label, href string, extraKeywords ...string) CommandItem {
	parts := make([]string, 0, 2+len(extraKeywords))
	parts = append(parts, group, label)
	parts = append(parts, extraKeywords...)
	return CommandItem{
		Group:    group,
		Label:    label,
		Href:     href,
		Keywords: keywords(parts...),
	}
}

// normalizeCommand fills Keywords when empty and drops unusable rows.
func normalizeCommand(it CommandItem) (CommandItem, bool) {
	if it.Href == "" || it.Label == "" {
		return CommandItem{}, false
	}
	if it.Keywords == "" {
		it.Keywords = keywords(it.Group, it.Label)
	}
	return it, true
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

	for _, it := range d.PluginCommands {
		if n, ok := normalizeCommand(it); ok {
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
			if n, ok := normalizeCommand(it); ok {
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
