package web

import (
	"sync"

	"github.com/lucasew/contapila-go/internal/plugin"
)

// Package-local registry for pages contributed through plugin.Reg.Page / Web middlemen.
var (
	pluginPagesMu sync.Mutex
	pluginPages   = NewPageRegistry()
	// pageModuleKey maps page slug → plugins map key (module ID).
	pageModuleKey = map[string]string{}
)

// scope is the Web middleman embedded in plugin.Reg. It only writes into
// this package's pluginPages registry, tagged with the module ID.
type scope struct {
	moduleID string
}

func (s *scope) ModuleID() string {
	if s == nil {
		return ""
	}
	return s.moduleID
}

// Page implements plugin.Web. page must be a web.Page.
func (s *scope) Page(page any) {
	if s == nil || s.moduleID == "" {
		return
	}
	p, ok := page.(Page)
	if !ok {
		panic("web: plugin.Web.Page expects web.Page")
	}
	if p.ID == "" {
		panic("web: page with empty ID")
	}
	if p.Body == nil {
		panic("web: page " + p.ID + " missing Body")
	}
	pluginPagesMu.Lock()
	defer pluginPagesMu.Unlock()
	if prev, ok := pageModuleKey[p.ID]; ok {
		// BindWeb re-runs setup; same module re-registering is a no-op.
		if prev == s.moduleID {
			return
		}
		panic("web: duplicate plugin page ID " + p.ID)
	}
	p.PluginKey = s.moduleID
	pluginPages.Register(p)
	pageModuleKey[p.ID] = s.moduleID
}

// BindPluginPages wires plugin setup to this package's local registry.
// Safe to call repeatedly; BindWeb runs once per process.
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
// and allows BindWeb to re-run setup hooks.
func resetPluginPagesForTest() {
	pluginPagesMu.Lock()
	pluginPages = NewPageRegistry()
	pageModuleKey = map[string]string{}
	pluginPagesMu.Unlock()
	plugin.ResetBindWebForTest()
}
