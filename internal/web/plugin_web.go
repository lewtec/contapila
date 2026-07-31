package web

import (
	"sync"

	"github.com/lucasew/contapila-go/internal/plugin"
)

// Package-local registry for pages contributed through plugin.Web middlemen.
var (
	pluginPagesMu sync.Mutex
	pluginPages   = NewPageRegistry()
	// pageModuleKey maps page slug → plugins map key (module ID).
	pageModuleKey = map[string]string{}
)

// scope is the middleman passed into plugin setup (reg.Web). It only writes into
// this package's pluginPages registry, tagged with the module ID.
type scope struct {
	moduleID string
	discard  bool // true when Web is nil (pre-bind setup)
}

func (s *scope) ModuleID() string {
	if s == nil {
		return ""
	}
	return s.moduleID
}

// Page registers a ledger report for this module (enable key = ModuleID).
func (s *scope) Page(p Page) {
	if s == nil || s.discard {
		return
	}
	if s.moduleID == "" {
		panic("web: Plugin scope missing module ID")
	}
	if p.ID == "" {
		panic("web: page with empty ID")
	}
	if p.Body == nil {
		panic("web: page " + p.ID + " missing Body")
	}
	pluginPagesMu.Lock()
	defer pluginPagesMu.Unlock()
	if _, ok := pageModuleKey[p.ID]; ok {
		// BindWeb re-runs setup; allow same module to re-register the same page.
		if pageModuleKey[p.ID] == s.moduleID {
			// Replace body/fill by re-registering: drop old index carefully — simplest skip.
			return
		}
		panic("web: duplicate plugin page ID " + p.ID)
	}
	p.PluginKey = s.moduleID
	pluginPages.Register(p)
	pageModuleKey[p.ID] = s.moduleID
}

// ScopeOf returns the host middleman for page registration.
// When Web is nil (setup before BindWeb), returns a discard scope so OnProcess-only
// modules can register at init without panicking.
func ScopeOf(w plugin.Web) *scope {
	if w == nil {
		return &scope{discard: true}
	}
	s, ok := w.(*scope)
	if !ok || s == nil {
		panic("web: ScopeOf expects middleman from web.BindPluginPages")
	}
	return s
}

// BindPluginPages wires plugin AttachWeb hooks to this package's local registry.
// Safe to call repeatedly; BindWeb runs attach once per process.
func BindPluginPages() {
	plugin.BindWeb(func(moduleID string) plugin.Web {
		return &scope{moduleID: moduleID}
	})
}

// pluginContributedPages returns pages registered via plugin middlemen (copy).
func pluginContributedPages() []Page {
	BindPluginPages()
	pluginPagesMu.Lock()
	defer pluginPagesMu.Unlock()
	if pluginPages == nil {
		return nil
	}
	out := make([]Page, len(pluginPages.pages))
	copy(out, pluginPages.pages)
	return out
}

// resetPluginPagesForTest clears the package-local plugin page registry
// and allows BindWeb to re-run AttachWeb hooks.
func resetPluginPagesForTest() {
	pluginPagesMu.Lock()
	pluginPages = NewPageRegistry()
	pageModuleKey = map[string]string{}
	pluginPagesMu.Unlock()
	plugin.ResetBindWebForTest()
}
