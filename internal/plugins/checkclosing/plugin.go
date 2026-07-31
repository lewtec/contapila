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

// Settings is plugins.check_closing.settings (none yet).
type Settings struct{}

func init() {
	plugin.RegisterTyped(plugin.TypedModule[Settings]{
		ID:   PluginKey,
		Book: book,
	})
}

func book(ctx plugin.BookContext, dirs []ast.Directive, _ Settings) (*booking.Engine, []ast.Directive, diag.List) {
	return booking.BookWithClosing(dirs, ctx.Setup)
}
