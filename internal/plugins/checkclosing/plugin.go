// Package checkclosing is the check_closing module: closing: TRUE autoclose.
package checkclosing

import (
	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/diag"
	"github.com/lucasew/contapila-go/internal/plugin"
)

// PluginKey is the journal plugin / CUE plugins map key.
const PluginKey = "check_closing"

// Options is plugins.check_closing (json tags ← cue.Value.Decode).
// enabled is read via config.PluginEnabled; kept here for Decode completeness.
type Options struct {
	Enabled bool `json:"enabled"`
}

func init() {
	plugin.RegisterTyped(plugin.TypedModule[Options]{
		ID:   PluginKey,
		Book: book,
	})
}

func book(ctx plugin.BookContext, dirs []ast.Directive, _ Options) (*booking.Engine, []ast.Directive, diag.List) {
	return booking.BookWithClosing(dirs, ctx.Setup)
}
