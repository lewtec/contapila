package web

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

// PagePluginKey is the default CUE plugins map key for a report slug.
// Example: page "accounts" → "web/accounts". Prefer Page.PluginKey from middleman.
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
		key := p.PluginKey
		if key == "" {
			key = PagePluginKey(p.ID)
		}
		if enabled != nil && !enabled(key) {
			continue
		}
		out.Register(p)
	}
	return out
}
