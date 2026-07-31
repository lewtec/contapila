// Package datedcosts expands {cost, date} postings into synthetic price directives.
package datedcosts

import (
	"iter"
	"slices"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/plugin"
)

// PluginKey is the journal plugin / CUE plugins map key.
const PluginKey = "dated_costs"

// Settings is plugins.dated_costs.settings (none yet).
type Settings struct{}

func init() {
	plugin.RegisterTyped(PluginKey, func(reg *plugin.Reg[Settings]) {
		reg.DefaultOn()
		reg.OnProcess(plugin.PhaseEarly, func(_ Settings, h *plugin.Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
			dirs := slices.Collect(in)
			out := booking.ExpandDatedCosts(dirs, h.Prices)
			return slices.Values(out)
		})
	})
}
