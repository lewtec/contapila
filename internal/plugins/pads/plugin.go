// Package pads materializes pad directives as synthetic balance-difference transactions.
package pads

import (
	"iter"
	"slices"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/plugin"
)

// PluginKey is the journal plugin / CUE plugins map key.
const PluginKey = "pads"

// Settings is plugins.pads.settings (none yet).
type Settings struct{}

func init() {
	plugin.RegisterTyped(PluginKey, func(reg *plugin.Reg[Settings]) {
		reg.DefaultOn()
		// PhaseBook before check_closing (registration order in cmd blank imports).
		reg.OnProcess(plugin.PhaseBook, func(_ Settings, h *plugin.Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
			dirs := slices.Collect(in)
			out, pdiags := booking.ExpandPads(dirs, h.Setup)
			h.Diags.Merge(pdiags)
			return slices.Values(out)
		})
	})
}
