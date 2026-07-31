// Package plugin hosts first-party modules.
//
// Options[S] is the CUE plugins.<id> shape: enabled + settings.
// The type parameter S is only settings; Book receives S (enabled is host-gated).
//
// AttachWeb receives a Web middleman from the host (package-local registry behind
// it). Plugins never call a global ContributePage.
package plugin

import (
	"fmt"
	"sort"
	"sync"

	"cuelang.org/go/cue"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/diag"
)

// Options is the common plugins.<id> value. S is module-specific settings only.
type Options[S any] struct {
	Enabled  bool `json:"enabled"`
	Settings S    `json:"settings"`
}

// Web is the middleman the host passes into AttachWeb. The host implements it
// with a package-local page registry; ModuleID is this module's plugins key.
type Web interface {
	ModuleID() string
}

// BookContext is the ledger-open context for stream/book modules.
type BookContext struct {
	Ledger string
	Root   string
	Setup  func(*booking.Engine)
}

// BookFunc is type-erased; opts is Options[S] as any (unwraps to S for TypedModule.Book).
type BookFunc func(ctx BookContext, dirs []ast.Directive, opts any) (*booking.Engine, []ast.Directive, diag.List)

// Module is one capability bundle (type-erased). Prefer RegisterTyped.
type Module struct {
	ID            string
	DecodeOptions func(v cue.Value) (any, error) // returns Options[S]
	Book          BookFunc
	attachWeb     func(Web) // set by RegisterTyped; run by BindWeb
}

// TypedModule registers a module with settings type S.
type TypedModule[S any] struct {
	ID string
	// Book is optional. Receives settings only.
	Book func(ctx BookContext, dirs []ast.Directive, settings S) (*booking.Engine, []ast.Directive, diag.List)
	// AttachWeb is optional. Invoked by the host via BindWeb with a middleman
	// scoped to this module (package-local registry on the host side).
	AttachWeb func(w Web)
}

var (
	mu      sync.RWMutex
	modules []Module
	byID    = map[string]int{}
)

// RegisterTyped adds a typed module and registers the journal plugin name.
// AttachWeb is stored and run later by BindWeb (not against a global page list).
func RegisterTyped[S any](m TypedModule[S]) {
	if m.ID == "" {
		panic("plugin: RegisterTyped with empty ID")
	}
	var book BookFunc
	if m.Book != nil {
		book = func(ctx BookContext, dirs []ast.Directive, opts any) (*booking.Engine, []ast.Directive, diag.List) {
			var settings S
			if opts != nil {
				if o, ok := opts.(Options[S]); ok {
					settings = o.Settings
				}
			}
			return m.Book(ctx, dirs, settings)
		}
	}
	Register(Module{
		ID: m.ID,
		DecodeOptions: func(v cue.Value) (any, error) {
			var out Options[S]
			if !v.Exists() {
				return out, nil
			}
			if err := v.Decode(&out); err != nil {
				return out, fmt.Errorf("plugin %s options: %w", m.ID, err)
			}
			return out, nil
		},
		Book:      book,
		attachWeb: m.AttachWeb,
	})
}

// Register adds a type-erased module. Prefer RegisterTyped.
func Register(m Module) {
	if m.ID == "" {
		panic("plugin: Register with empty ID")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := byID[m.ID]; dup {
		panic("plugin: duplicate module ID " + m.ID)
	}
	byID[m.ID] = len(modules)
	modules = append(modules, m)
	config.RegisterKnownPlugin(m.ID)
}

// BindWeb runs each module's AttachWeb with a middleman from newWeb(moduleID).
// The host (web package) owns the registry; newWeb returns a scoped middleman.
// Safe to call multiple times; AttachWeb runs once per module per process.
func BindWeb(newWeb func(moduleID string) Web) {
	bindWebOnce.Do(func() {
		if newWeb == nil {
			return
		}
		for _, m := range All() {
			if m.attachWeb == nil {
				continue
			}
			m.attachWeb(newWeb(m.ID))
		}
	})
}

var bindWebOnce sync.Once

// All returns modules in registration order.
func All() []Module {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Module, len(modules))
	copy(out, modules)
	return out
}

// PluginValue returns plugins.<id> from unified config.
func PluginValue(cfg cue.Value, id string) cue.Value {
	if !cfg.Exists() || id == "" {
		return cue.Value{}
	}
	return cfg.LookupPath(cue.MakePath(cue.Str("plugins"), cue.Str(id)))
}

// EnabledBookModules returns Book modules with plugins.<id>.enabled true.
func EnabledBookModules(cfg cue.Value) []Module {
	var out []Module
	for _, m := range All() {
		if m.Book == nil {
			continue
		}
		on, err := config.PluginEnabled(cfg, m.ID)
		if err != nil || !on {
			continue
		}
		out = append(out, m)
	}
	return out
}

// DecodeOptions decodes plugins.<id> into Options[S] (as any).
func DecodeOptions(cfg cue.Value, m Module) (any, error) {
	if m.DecodeOptions == nil {
		return nil, nil
	}
	return m.DecodeOptions(PluginValue(cfg, m.ID))
}

// ValidateOptions decodes every present plugins.<id> for registered modules.
func ValidateOptions(cfg cue.Value) error {
	for _, m := range All() {
		v := PluginValue(cfg, m.ID)
		if !v.Exists() || m.DecodeOptions == nil {
			continue
		}
		if _, err := m.DecodeOptions(v); err != nil {
			return err
		}
	}
	return nil
}

// IDs returns sorted module ids.
func IDs() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(modules))
	for _, m := range modules {
		out = append(out, m.ID)
	}
	sort.Strings(out)
	return out
}

func resetForTest() {
	mu.Lock()
	defer mu.Unlock()
	modules = nil
	byID = map[string]int{}
	bindWebOnce = sync.Once{}
}

// ResetBindWebForTest allows BindWeb to run again after clearing host registries.
func ResetBindWebForTest() {
	bindWebOnce = sync.Once{}
}
