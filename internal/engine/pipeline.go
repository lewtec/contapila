package engine

import (
	"fmt"
	"log/slog"
	"strings"

	"cuelang.org/go/cue"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/diag"
)

// moduleOn is CUE explicit flag, else journal plugin "id" in this ledger, else defaultOn.
func moduleOn(cfg cue.Value, journal map[string]bool, id string, defaultOn bool) bool {
	if flags, err := config.PluginFlags(cfg); err == nil && flags != nil {
		if b, ok := flags[id]; ok {
			return b
		}
	}
	if journal[id] {
		return true
	}
	return defaultOn
}

// journalPlugins collects plugin "name" directives from an already-loaded stream.
// Unknown names and ids with '/' are skipped with a warn diag.
func journalPlugins(dirs []ast.Directive) (map[string]bool, diag.List) {
	on := map[string]bool{}
	seen := map[string]bool{}
	var diags diag.List
	for _, d := range dirs {
		p, ok := d.(ast.Plugin)
		if !ok || p.Name == "" {
			continue
		}
		if err := config.ValidatePluginID(p.Name); err != nil {
			if !seen[p.Name] {
				seen[p.Name] = true
				diags.Warn(p.File, p.Line, fmt.Sprintf("invalid plugin id %q: '/' not allowed", p.Name))
				slog.Warn("invalid plugin directive", "plugin", p.Name, "file", p.File, "line", p.Line, "err", err.Error())
			}
			continue
		}
		if !config.IsKnownPlugin(p.Name) {
			if !seen[p.Name] {
				seen[p.Name] = true
				msg := fmt.Sprintf("unknown plugin %q", p.Name)
				if known := config.KnownPluginIDs(); len(known) > 0 {
					msg = fmt.Sprintf("unknown plugin %q (known: %s)", p.Name, strings.Join(known, ", "))
				}
				diags.Warn(p.File, p.Line, msg)
				slog.Warn("unknown plugin directive", "plugin", p.Name, "file", p.File, "line", p.Line)
			}
			continue
		}
		on[p.Name] = true
	}
	return on, diags
}
