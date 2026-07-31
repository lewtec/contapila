// Package autointerest expands interest_rate opens into income pads/balances.
package autointerest

import (
	"iter"
	"slices"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/plugin"
)

// PluginKey is the journal plugin / CUE plugins map key.
const PluginKey = "autointerest"

// Settings is plugins.autointerest.settings (none yet).
type Settings struct{}

func init() {
	plugin.RegisterTyped(PluginKey, func(reg *plugin.Reg[Settings]) {
		reg.DefaultOn()
		reg.OnProcess(plugin.PhaseMid, func(_ Settings, h *plugin.Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
			dirs := slices.Collect(in)
			h.Diags.Merge(booking.ValidateAutoInterestRates(dirs))
			out, adiags := booking.ExpandAutoInterest(dirs)
			h.Diags.Merge(adiags)
			return slices.Values(out)
		})
	})
}
