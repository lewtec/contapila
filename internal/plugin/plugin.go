// Package plugin hosts first-party modules.
//
//	plugin.RegisterTyped("check_closing", func(reg *plugin.Reg[Settings]) {
//	    reg.OnProcess(func(settings Settings, h *plugin.Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
//	        ...
//	    })
//	})
//
//	plugin.RegisterTyped("web/accounts", func(reg *plugin.Reg[Settings]) {
//	    web.ScopeOf(reg.Web).Page(...)
//	})
//
// S is settings only (plugins.<id>.settings). enabled is host-gated.
// Reg is the scoped middleman (Web + OnProcess).
package plugin

import (
	"fmt"
	"iter"
	"slices"
	"sort"
	"sync"

	"cuelang.org/go/cue"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/diag"
)

// Web is the host middleman for UI registration (package-local registry on the host).
type Web interface {
	ModuleID() string
}

// Host is per-ledger-open context for stream processing.
type Host struct {
	Ledger string
	Root   string
	Setup  func(*booking.Engine)
	// Engine/Diags may be set by OnProcess when the module owns booking.
	Engine *booking.Engine
	Diags  diag.List
}

// ProcessFunc transforms a directive stream (Go iterator).
type ProcessFunc func(settings any, h *Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive]

// Reg is the scoped handle passed into the plugin setup function.
type Reg[S any] struct {
	ModuleID string
	Web      Web // nil until host BindWeb; page registration no-ops until then

	onProcess func(settings S, h *Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive]
}

// OnProcess registers the stream transformer. settings come from plugins.<id>.settings.
func (r *Reg[S]) OnProcess(fn func(settings S, h *Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive]) {
	if r == nil {
		return
	}
	r.onProcess = fn
}

type module struct {
	id      string
	setup   func(w Web)
	decode  func(v cue.Value) (any, error)
	process ProcessFunc
}

var (
	mu       sync.RWMutex
	modules  []module
	byID     = map[string]int{}
	bindOnce sync.Once
)

// RegisterTyped registers a module. setup receives scoped Reg; call from init.
// setup runs immediately with Web=nil (captures OnProcess) and again on BindWeb with a real Web.
func RegisterTyped[S any](id string, setup func(reg *Reg[S])) {
	if id == "" {
		panic("plugin: RegisterTyped with empty ID")
	}
	if setup == nil {
		panic("plugin: RegisterTyped nil setup")
	}

	apply := func(w Web) {
		reg := &Reg[S]{ModuleID: id, Web: w}
		setup(reg)
		if reg.onProcess == nil {
			return
		}
		fn := reg.onProcess
		mu.Lock()
		defer mu.Unlock()
		if i, ok := byID[id]; ok {
			modules[i].process = func(settings any, h *Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
				var s S
				if settings != nil {
					if typed, ok := settings.(S); ok {
						s = typed
					}
				}
				return fn(s, h, in)
			}
		}
	}

	mu.Lock()
	if _, dup := byID[id]; dup {
		mu.Unlock()
		panic("plugin: duplicate module ID " + id)
	}
	m := module{
		id: id,
		decode: func(v cue.Value) (any, error) {
			var s S
			if !v.Exists() {
				return s, nil
			}
			sv := v.LookupPath(cue.ParsePath("settings"))
			if !sv.Exists() {
				return s, nil
			}
			if err := sv.Decode(&s); err != nil {
				return s, fmt.Errorf("plugin %s settings: %w", id, err)
			}
			return s, nil
		},
		setup: apply,
	}
	byID[id] = len(modules)
	modules = append(modules, m)
	mu.Unlock()

	config.RegisterKnownPlugin(id)
	// Capture OnProcess before any host bind (engine path).
	apply(nil)
}

// BindWeb runs each module's setup again with a scoped Web middleman (pages).
func BindWeb(newWeb func(moduleID string) Web) {
	bindOnce.Do(func() {
		if newWeb == nil {
			return
		}
		mu.RLock()
		list := slices.Clone(modules)
		mu.RUnlock()
		for _, m := range list {
			if m.setup != nil {
				m.setup(newWeb(m.id))
			}
		}
	})
}

// EnabledStreamModules returns modules that registered OnProcess and are enabled.
func EnabledStreamModules(cfg cue.Value) []module {
	mu.RLock()
	defer mu.RUnlock()
	var out []module
	for _, m := range modules {
		if m.process == nil {
			continue
		}
		on, err := config.PluginEnabled(cfg, m.id)
		if err != nil || !on {
			continue
		}
		out = append(out, m)
	}
	return out
}

// RunStream runs enabled stream modules in order over dirs.
// If a module sets h.Engine, that engine is returned (last writer wins).
func RunStream(cfg cue.Value, h *Host, dirs []ast.Directive) (e *booking.Engine, out []ast.Directive, diags diag.List) {
	out = dirs
	if h == nil {
		h = &Host{}
	}
	for _, m := range EnabledStreamModules(cfg) {
		settings, err := m.decode(PluginValue(cfg, m.id))
		if err != nil {
			diags.Error("", 0, err.Error())
			continue
		}
		h.Diags = nil
		seq := m.process(settings, h, slices.Values(out))
		out = slices.Collect(seq)
		if h.Engine != nil {
			e = h.Engine
		}
		diags.Merge(h.Diags)
	}
	return e, out, diags
}

// PluginValue returns plugins.<id> from unified config.
func PluginValue(cfg cue.Value, id string) cue.Value {
	if !cfg.Exists() || id == "" {
		return cue.Value{}
	}
	return cfg.LookupPath(cue.MakePath(cue.Str("plugins"), cue.Str(id)))
}

// ValidateOptions decodes settings for every present plugins.<id>.
func ValidateOptions(cfg cue.Value) error {
	mu.RLock()
	defer mu.RUnlock()
	for _, m := range modules {
		v := PluginValue(cfg, m.id)
		if !v.Exists() {
			continue
		}
		if _, err := m.decode(v); err != nil {
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
		out = append(out, m.id)
	}
	sort.Strings(out)
	return out
}

func resetForTest() {
	mu.Lock()
	defer mu.Unlock()
	modules = nil
	byID = map[string]int{}
	bindOnce = sync.Once{}
}

// ResetBindWebForTest allows BindWeb to run setup again after clearing host state.
func ResetBindWebForTest() {
	bindOnce = sync.Once{}
}
