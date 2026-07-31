// Package checkclosing is the check_closing module: closing: TRUE autoclose.
package checkclosing

import (
	"iter"
	"slices"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/plugin"
)

// PluginKey is the journal plugin / CUE plugins map key.
const PluginKey = "check_closing"

// Settings is plugins.check_closing.settings (none yet).
type Settings struct{}

func init() {
	plugin.RegisterTyped(PluginKey, func(reg *plugin.Reg[Settings]) {
		reg.OnProcess(func(_ Settings, h *plugin.Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
			dirs := slices.Collect(in)
			e, out, diags := booking.BookWithClosing(dirs, h.Setup)
			h.Engine = e
			h.Diags = diags
			return slices.Values(out)
		})
	})
}
