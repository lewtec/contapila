// Package plugin hosts first-party modules. One RegisterTyped can attach typed
// options (Go struct + cue.Value.Decode), a stream booker, and/or web setup.
//
// Options are plain Go structs with json tags; CUE fills them via Decode
// (same interop as the rest of contapila). Prelude #Plugin supplies enabled.
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

// BookContext is the ledger-open context for stream/book modules.
type BookContext struct {
	Ledger string
	Root   string
	Setup  func(*booking.Engine)
}

// BookFunc is a type-erased booker. opts is the module's decoded options.
type BookFunc func(ctx BookContext, dirs []ast.Directive, opts any) (*booking.Engine, []ast.Directive, diag.List)

// Module is one capability bundle (type-erased). Prefer RegisterTyped.
type Module struct {
	ID            string
	DecodeOptions func(v cue.Value) (any, error)
	Book          BookFunc
	AttachWeb     func()
}

// TypedModule registers a module with options type T (json tags + cue Decode).
type TypedModule[T any] struct {
	ID string
	// Book is optional (e.g. check_closing).
	Book func(ctx BookContext, dirs []ast.Directive, opts T) (*booking.Engine, []ast.Directive, diag.List)
	// AttachWeb is optional (e.g. ContributePage).
	AttachWeb func()
}

var (
	mu      sync.RWMutex
	modules []Module
	byID    = map[string]int{}
	webOnce sync.Once
)

// RegisterTyped adds a typed module and registers its journal plugin name.
func RegisterTyped[T any](m TypedModule[T]) {
	if m.ID == "" {
		panic("plugin: RegisterTyped with empty ID")
	}
	var book BookFunc
	if m.Book != nil {
		book = func(ctx BookContext, dirs []ast.Directive, opts any) (*booking.Engine, []ast.Directive, diag.List) {
			var t T
			if opts != nil {
				if typed, ok := opts.(T); ok {
					t = typed
				}
			}
			return m.Book(ctx, dirs, t)
		}
	}
	Register(Module{
		ID: m.ID,
		DecodeOptions: func(v cue.Value) (any, error) {
			var out T
			if !v.Exists() {
				return out, nil
			}
			// Decode uses json tags / CUE–Go field mapping (same as config decode elsewhere).
			if err := v.Decode(&out); err != nil {
				return out, fmt.Errorf("plugin %s options: %w", m.ID, err)
			}
			return out, nil
		},
		Book:      book,
		AttachWeb: m.AttachWeb,
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

// All returns modules in registration order.
func All() []Module {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Module, len(modules))
	copy(out, modules)
	return out
}

// AttachAllWeb runs each module's AttachWeb once (web host).
func AttachAllWeb() {
	webOnce.Do(func() {
		for _, m := range All() {
			if m.AttachWeb != nil {
				m.AttachWeb()
			}
		}
	})
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

// DecodeOptions decodes plugins.<id> into the module's options type.
func DecodeOptions(cfg cue.Value, m Module) (any, error) {
	if m.DecodeOptions == nil {
		return nil, nil
	}
	return m.DecodeOptions(PluginValue(cfg, m.ID))
}

// ValidateOptions decodes every present plugins.<id> entry for registered modules.
// Call after config load to surface bad option types early.
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
	webOnce = sync.Once{}
}
