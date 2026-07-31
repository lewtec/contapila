package config

import (
	"sort"
	"sync"
)

// Known first-party module IDs (e.g. "web_accounts"). Registered from init()
// via RegisterTyped / pages. Used to warn on journal plugin "…" that name
// nothing in this binary. IDs must not contain '/'.

var (
	knownPluginMu sync.RWMutex
	knownPlugins  = map[string]struct{}{}
)

// RegisterKnownPlugin records a module id shipped in this binary. Safe for init().
// Empty id is ignored. IDs with '/' panic. Duplicate registration is a no-op.
func RegisterKnownPlugin(id string) {
	if id == "" {
		return
	}
	if err := ValidatePluginID(id); err != nil {
		panic("plugin: " + err.Error())
	}
	knownPluginMu.Lock()
	defer knownPluginMu.Unlock()
	knownPlugins[id] = struct{}{}
}

// IsKnownPlugin reports whether id is a registered first-party module.
func IsKnownPlugin(id string) bool {
	knownPluginMu.RLock()
	defer knownPluginMu.RUnlock()
	_, ok := knownPlugins[id]
	return ok
}

// KnownPluginIDs returns registered module ids in sorted order.
func KnownPluginIDs() []string {
	knownPluginMu.RLock()
	defer knownPluginMu.RUnlock()
	out := make([]string, 0, len(knownPlugins))
	for id := range knownPlugins {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// resetKnownPluginsForTest clears the known set (tests only).
func resetKnownPluginsForTest() {
	knownPluginMu.Lock()
	defer knownPluginMu.Unlock()
	knownPlugins = map[string]struct{}{}
}
