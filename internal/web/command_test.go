package web

import (
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/lucasew/contapila-go/internal/engine"
)

func TestCommandItemsHomeLedgersOnly(t *testing.T) {
	d := pageData{
		Page:    "home",
		Ledgers: []string{"personal", "acme"},
	}
	items := commandItems(d)
	if len(items) != 2 {
		t.Fatalf("want 2 ledger items, got %d %+v", len(items), items)
	}
	for _, it := range items {
		if it.Group != "Ledgers" {
			t.Fatalf("group %q", it.Group)
		}
		if !strings.HasPrefix(it.Href, "/l/") {
			t.Fatalf("href %q", it.Href)
		}
	}
}

func TestCommandItemsLedgerReportsQueriesAccounts(t *testing.T) {
	r := NewPageRegistry()
	body := func(d PageData) templ.Component { return templ.NopComponent }
	r.Register(Page{ID: "check", Label: "Check", Order: 10, Sidebar: true, Body: body})
	r.Register(Page{ID: "balances", Label: "Balances", Order: 20, Sidebar: true, Body: body})
	r.Register(Page{ID: "queries", Label: "Queries", Order: 65, Section: "Queries", Sidebar: false, Body: body})

	d := pageData{
		Pages:           r,
		Page:            "check",
		LedgerName:      "personal",
		Ledgers:         []string{"personal", "acme"},
		Time:            "2024",
		QueryNav:        []QueryNavItem{{Name: "cash"}},
		JumpAccounts:    []string{"Assets:Cash", "Expenses:Food"},
		JumpCommodities: []string{"BRL", "USD"},
	}
	items := commandItems(d)

	byGroup := map[string][]CommandItem{}
	for _, it := range items {
		byGroup[it.Group] = append(byGroup[it.Group], it)
	}
	if len(byGroup["Ledgers"]) != 2 {
		t.Fatalf("ledgers: %+v", byGroup["Ledgers"])
	}
	if len(byGroup["Reports"]) != 2 {
		t.Fatalf("reports: %+v", byGroup["Reports"])
	}
	if len(byGroup["Queries"]) != 1 || byGroup["Queries"][0].Label != "cash" {
		t.Fatalf("queries: %+v", byGroup["Queries"])
	}
	if len(byGroup["Accounts"]) != 2 {
		t.Fatalf("accounts: %+v", byGroup["Accounts"])
	}
	if len(byGroup["Commodities"]) != 2 {
		t.Fatalf("commodities: %+v", byGroup["Commodities"])
	}

	// Preserve time on report jump.
	for _, it := range byGroup["Reports"] {
		if !strings.Contains(it.Href, "time=2024") {
			t.Fatalf("report href missing time: %q", it.Href)
		}
	}
	// Account keywords expand colon segments.
	for _, it := range byGroup["Accounts"] {
		if !strings.Contains(it.Keywords, "cash") && !strings.Contains(it.Keywords, "food") {
			t.Fatalf("keywords %q", it.Keywords)
		}
	}
}

func TestLedgerSwitchHrefEntityFallback(t *testing.T) {
	d := pageData{Page: "account", LedgerName: "personal", Time: "2024"}
	href := ledgerSwitchHref(d, "acme")
	if href != "/l/acme/check?time=2024" {
		t.Fatalf("got %q", href)
	}
	d.Page = "pnl"
	href = ledgerSwitchHref(d, "acme")
	if href != "/l/acme/pnl?time=2024" {
		t.Fatalf("got %q", href)
	}
}

func TestJumpNamesSorted(t *testing.T) {
	l := &engine.Ledger{
		Accounts: map[string]engine.AccountInfo{
			"Expenses:Food": {},
			"Assets:Cash":   {},
		},
		Commodities: map[string]engine.CommodityInfo{
			"USD": {},
			"BRL": {},
		},
	}
	accts := jumpAccountNames(l)
	if len(accts) != 2 || accts[0] != "Assets:Cash" {
		t.Fatalf("accounts %v", accts)
	}
	comms := jumpCommodityNames(l)
	if len(comms) != 2 || comms[0] != "BRL" {
		t.Fatalf("commodities %v", comms)
	}
}

func TestCommandItemsStaticHrefs(t *testing.T) {
	sess := &Session{Static: true}
	d := pageData{
		Sess:       sess,
		Page:       "check",
		LedgerName: "personal",
		Ledgers:    []string{"personal"},
	}
	r := NewPageRegistry()
	r.Register(Page{
		ID: "check", Label: "Check", Order: 10, Sidebar: true,
		Body: func(d PageData) templ.Component { return templ.NopComponent },
	})
	d.Pages = r
	items := commandItems(d)
	for _, it := range items {
		if !strings.HasSuffix(it.Href, ".html") {
			t.Fatalf("static href %q", it.Href)
		}
	}
}

func TestCommandItemsIncludesPluginCommands(t *testing.T) {
	d := pageData{
		Page:       "check",
		LedgerName: "personal",
		Ledgers:    []string{"personal"},
		PluginCommands: []CommandItem{
			Command("Extra", "Thing", "/l/personal/thing", "thing"),
			{Label: "skip-no-href"}, // dropped
		},
	}
	items := commandItems(d)
	var found bool
	for _, it := range items {
		if it.Group == "Extra" && it.Label == "Thing" {
			found = true
			if it.Keywords == "" {
				t.Fatal("expected keywords")
			}
		}
		if it.Label == "skip-no-href" {
			t.Fatal("empty href should be dropped")
		}
	}
	if !found {
		t.Fatalf("plugin command missing: %+v", items)
	}
}

func TestPluginContribEnabled(t *testing.T) {
	if !pluginContribEnabled("x", true, nil) {
		t.Fatal("default on, no flags")
	}
	if pluginContribEnabled("x", false, nil) {
		t.Fatal("default off, no flags")
	}
	if !pluginContribEnabled("x", false, map[string]bool{"x": true}) {
		t.Fatal("explicit on")
	}
	if pluginContribEnabled("x", true, map[string]bool{"x": false}) {
		t.Fatal("explicit off")
	}
}

func TestApplyPaletteEntriesRespectsFlags(t *testing.T) {
	entries := []paletteEntry{
		{
			pluginKey:      "web_demo",
			defaultEnabled: false,
			fill: func(pc PageContext) []CommandItem {
				return []CommandItem{Command("Demo", "Hit", "/hit")}
			},
		},
		{
			pluginKey:      "web_on",
			defaultEnabled: true,
			fill: func(pc PageContext) []CommandItem {
				return []CommandItem{Command("On", "Always", "/always")}
			},
		},
	}
	pc := PageContext{LedgerName: "personal"}

	// No flags: only default-on modules.
	got := applyPaletteEntries(entries, nil, pc)
	if len(got) != 1 || got[0].Label != "Always" {
		t.Fatalf("default-on only: %+v", got)
	}

	// Explicit enable demo, disable on.
	got = applyPaletteEntries(entries, map[string]bool{
		"web_demo": true,
		"web_on":   false,
	}, pc)
	if len(got) != 1 || got[0].Label != "Hit" {
		t.Fatalf("explicit flags: %+v", got)
	}
}

func TestCommandHelperKeywords(t *testing.T) {
	it := Command("Accounts", "Assets:Cash", "/a", "account")
	if !strings.Contains(it.Keywords, "cash") {
		t.Fatalf("keywords %q", it.Keywords)
	}
}
