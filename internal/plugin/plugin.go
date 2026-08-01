// Package plugin hosts first-party modules.
//
//	plugin.RegisterTyped("dated_costs", func(reg *plugin.Reg[Settings]) {
//	    reg.DefaultOn()
//	    reg.OnProcess(plugin.PhaseEarly, func(settings Settings, h *plugin.Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
//	        ...
//	    })
//	})
//
// S is settings only (plugins.<id>.settings). enabled is host-gated; DefaultOn() for Helix defaults.
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
	"github.com/lucasew/contapila-go/internal/prices"
)

// Phase orders stream processors in the ledger open pipeline.
type Phase int

const (
	// PhaseEarly: after stream inject, before mid expanders (dated_costs).
	PhaseEarly Phase = iota
	// PhaseMid: stream rewrite before opens are collected (autointerest expand).
	PhaseMid
	// PhaseBook: around booking; Host.Setup set (pads, check_closing).
	PhaseBook
	// PhaseLate: after book; docs side-effects (docs_folder, docs_meta).
	PhaseLate
)

// Web is the host middleman for UI registration (package-local registry on the host).
type Web interface {
	ModuleID() string
	Page(page any)
}

// Host is per-ledger-open context for stream processing.
type Host struct {
	Ledger string
	Root   string
	Setup  func(*booking.Engine)
	Prices *prices.DB
	// Documents accumulates synth docs from late plugins (docs_folder, docs_meta).
	Documents []ast.Document
	// Engine/Diags may be set by PhaseBook processors that own booking.
	Engine *booking.Engine
	Diags  diag.List
}

// ProcessFunc transforms a directive stream.
type ProcessFunc func(settings any, h *Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive]

// Reg is the scoped handle passed into the plugin setup function.
type Reg[S any] struct {
	ModuleID  string
	Web       Web
	defaultOn bool
	// phase → handler (last wins per phase)
	onProcess map[Phase]func(settings S, h *Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive]
}

// DefaultOn marks the module enabled when CUE/journal omit plugins.<id>.
func (r *Reg[S]) DefaultOn() {
	if r != nil {
		r.defaultOn = true
	}
}

// Page registers a ledger report via the host middleman (no-op if Web not bound).
func (r *Reg[S]) Page(page any) {
	if r == nil || r.Web == nil || page == nil {
		return
	}
	r.Web.Page(page)
}

// OnProcess registers a stream transformer for phase.
func (r *Reg[S]) OnProcess(phase Phase, fn func(settings S, h *Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive]) {
	if r == nil || fn == nil {
		return
	}
	if r.onProcess == nil {
		r.onProcess = map[Phase]func(settings S, h *Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive]{}
	}
	r.onProcess[phase] = fn
}

type module struct {
	id        string
	setup     func(w Web)
	decode    func(v cue.Value) (any, error)
	defaultOn bool
	// processes by phase (filled after setup)
	processes map[Phase]ProcessFunc
}

var (
	mu       sync.RWMutex
	modules  []module
	byID     = map[string]int{}
	bindOnce sync.Once
)

// RegisterTyped registers a module. setup receives scoped Reg; call from init.
// id must be a valid plugin id (no '/'; see config.ValidatePluginID).
func RegisterTyped[S any](id string, setup func(reg *Reg[S])) {
	if err := config.ValidatePluginID(id); err != nil {
		panic("plugin: RegisterTyped: " + err.Error())
	}
	if setup == nil {
		panic("plugin: RegisterTyped nil setup")
	}

	apply := func(w Web) {
		reg := &Reg[S]{ModuleID: id, Web: w}
		setup(reg)
		mu.Lock()
		defer mu.Unlock()
		i, ok := byID[id]
		if !ok {
			return
		}
		if reg.defaultOn {
			modules[i].defaultOn = true
		}
		if modules[i].processes == nil {
			modules[i].processes = map[Phase]ProcessFunc{}
		}
		for phase, fn := range reg.onProcess {
			fn := fn
			modules[i].processes[phase] = func(settings any, h *Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
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
		setup:     apply,
		processes: map[Phase]ProcessFunc{},
	}
	byID[id] = len(modules)
	modules = append(modules, m)
	mu.Unlock()

	config.RegisterKnownPlugin(id)
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

func isEnabled(cfg cue.Value, m module) bool {
	flags, err := config.PluginFlags(cfg)
	if err != nil {
		return m.defaultOn
	}
	if flags != nil {
		if b, ok := flags[m.id]; ok {
			return b
		}
	}
	return m.defaultOn
}

// RunPhase runs enabled processors for one phase in registration order.
// If a processor sets h.Engine, that engine is returned (last writer wins).
func RunPhase(cfg cue.Value, h *Host, dirs []ast.Directive, phase Phase) (e *booking.Engine, out []ast.Directive, diags diag.List) {
	out = dirs
	if h == nil {
		h = &Host{}
	}
	mu.RLock()
	list := slices.Clone(modules)
	mu.RUnlock()

	for _, m := range list {
		fn := m.processes[phase]
		if fn == nil {
			continue
		}
		if !cfg.Exists() {
			if !m.defaultOn {
				continue
			}
		} else if !isEnabled(cfg, m) {
			continue
		}
		settings, err := m.decode(PluginValue(cfg, m.id))
		if err != nil {
			diags.Error("", 0, err.Error())
			continue
		}
		// Isolate this module's contribution; plugins must Merge into h.Diags
		// (never assign), then we harvest into the phase list.
		h.Diags = nil
		seq := fn(settings, h, slices.Values(out))
		out = slices.Collect(seq)
		if h.Engine != nil {
			e = h.Engine
		}
		diags.Merge(h.Diags)
	}
	return e, out, diags
}

// RunStream runs all phases Early→Mid→Book→Late (convenience / tests).
// The last non-nil Engine from any phase is returned (PhaseLate must not clear it).
func RunStream(cfg cue.Value, h *Host, dirs []ast.Directive) (e *booking.Engine, out []ast.Directive, diags diag.List) {
	out = dirs
	if h == nil {
		h = &Host{}
	}
	for _, phase := range []Phase{PhaseEarly, PhaseMid, PhaseBook, PhaseLate} {
		var pd diag.List
		var pe *booking.Engine
		pe, out, pd = RunPhase(cfg, h, out, phase)
		diags.Merge(pd)
		if pe != nil {
			e = pe
		}
	}
	if e == nil && h.Engine != nil {
		e = h.Engine
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

// Info is a debug snapshot of one registered module.
type Info struct {
	ID        string
	HasStream bool
	DefaultOn bool
	Phases    []string
}

// Infos returns registered modules in sorted id order (for debug UI).
func Infos() []Info {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Info, 0, len(modules))
	for _, m := range modules {
		info := Info{ID: m.id, DefaultOn: m.defaultOn, HasStream: len(m.processes) > 0}
		for ph := range m.processes {
			info.Phases = append(info.Phases, phaseName(ph))
		}
		sort.Strings(info.Phases)
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func phaseName(p Phase) string {
	switch p {
	case PhaseEarly:
		return "early"
	case PhaseMid:
		return "mid"
	case PhaseBook:
		return "book"
	case PhaseLate:
		return "late"
	default:
		return fmt.Sprintf("phase(%d)", p)
	}
}

// IsEnabled reports whether module id is on for cfg (explicit flag or DefaultOn).
func IsEnabled(cfg cue.Value, id string) bool {
	mu.RLock()
	i, ok := byID[id]
	var m module
	if ok {
		m = modules[i]
	}
	mu.RUnlock()
	if !ok {
		on, _ := config.PluginEnabled(cfg, id)
		return on
	}
	return isEnabled(cfg, m)
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
