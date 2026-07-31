// Package plugin hosts first-party modules.
//
// Options[S] is the CUE plugins.<id> shape: enabled + settings.
// The type parameter S is only settings; Book handlers receive S, not Options.
//
// AttachWeb is a field on the module definition; RegisterTyped runs it once
// when the plugin package registers (plugin-scoped init), not a host registry.
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

// BookContext is the ledger-open context for stream/book modules.
type BookContext struct {
	Ledger string
	Root   string
	Setup  func(*booking.Engine)
}

// BookFunc is type-erased; opts is Options[S] as any (Book unwraps to S).
type BookFunc func(ctx BookContext, dirs []ast.Directive, opts any) (*booking.Engine, []ast.Directive, diag.List)

// Module is one capability bundle (type-erased). Prefer RegisterTyped.
type Module struct {
	ID            string
	DecodeOptions func(v cue.Value) (any, error) // returns Options[S]
	Book          BookFunc
}

// TypedModule registers a module with settings type S.
type TypedModule[S any] struct {
	ID string
	// Book is optional. Receives settings only (enabled is host-gated).
	Book func(ctx BookContext, dirs []ast.Directive, settings S) (*booking.Engine, []ast.Directive, diag.List)
	// AttachWeb is optional. Run once at RegisterTyped (from the plugin package's
	// init path) so web setup stays scoped to this module definition.
	AttachWeb func()
}

var (
	mu      sync.RWMutex
	modules []Module
	byID    = map[string]int{}
)

// RegisterTyped adds a typed module, registers the journal plugin name, and
// runs AttachWeb if set (still during the plugin package's init).
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
		Book: book,
	})
	if m.AttachWeb != nil {
		m.AttachWeb()
	}
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
}
