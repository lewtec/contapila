package web

import (
	"sync"
)

// ContributePage registers a first-party ledger report for composition into
// Server registries (sidebar + route + build). Safe from init().
// Modules call this from their own init (alongside plugin.RegisterTyped).
// Does not mutate DefaultPages(); extras are applied via ComposePages.
// Plugin enable keys use PagePluginKey (web/{ID}); register that name via
// plugin.RegisterTyped, not here.
func ContributePage(p Page) {
	if p.ID == "" {
		panic("web: ContributePage with empty ID")
	}
	if p.Body == nil {
		panic("web: ContributePage " + p.ID + " missing Body")
	}
	contribMu.Lock()
	defer contribMu.Unlock()
	for _, e := range contribPages {
		if e.ID == p.ID {
			panic("web: ContributePage duplicate ID " + p.ID)
		}
	}
	contribPages = append(contribPages, p)
}

// ContributedPages returns a copy of pages registered via ContributePage.
func ContributedPages() []Page {
	contribMu.Lock()
	defer contribMu.Unlock()
	if len(contribPages) == 0 {
		return nil
	}
	out := make([]Page, len(contribPages))
	copy(out, contribPages)
	return out
}

var (
	contribMu    sync.Mutex
	contribPages []Page
)

// Clone returns a new registry with the same pages (Fill/Body funcs shared).
// Nil receiver yields an empty registry.
func (r *PageRegistry) Clone() *PageRegistry {
	out := NewPageRegistry()
	if r == nil {
		return out
	}
	for _, p := range r.pages {
		out.Register(p)
	}
	return out
}

// ComposePages clones base (or empty if nil) and registers extra pages in order.
// Panics on duplicate IDs (including clashes with base).
func ComposePages(base *PageRegistry, extra ...Page) *PageRegistry {
	out := base.Clone()
	for _, p := range extra {
		out.Register(p)
	}
	return out
}

// PagePluginKey is the CUE plugins map key for a ledger report slug.
// Example: page "accounts" → "web/accounts".
func PagePluginKey(pageID string) string {
	return "web/" + pageID
}

// FilterPagesByPlugins returns a new registry keeping only pages whose
// plugin key is enabled. enabled(key) false drops the page; nil enabled keeps all.
func FilterPagesByPlugins(r *PageRegistry, enabled func(pluginKey string) bool) *PageRegistry {
	out := NewPageRegistry()
	if r == nil {
		return out
	}
	for _, p := range r.pages {
		if enabled != nil && !enabled(PagePluginKey(p.ID)) {
			continue
		}
		out.Register(p)
	}
	return out
}

// resetContributedPagesForTest clears ContributePage state (tests only).
func resetContributedPagesForTest() {
	contribMu.Lock()
	defer contribMu.Unlock()
	contribPages = nil
}
