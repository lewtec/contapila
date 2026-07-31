// Package docsfolder synthesizes document directives from <ledger>/docs/by-account.
package docsfolder

import (
	"iter"
	"log/slog"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/docs"
	"github.com/lucasew/contapila-go/internal/plugin"
)

// PluginKey is the journal plugin / CUE plugins map key.
const PluginKey = "docs_folder"

// Settings is plugins.docs_folder.settings (none yet).
type Settings struct{}

func init() {
	plugin.RegisterTyped(PluginKey, func(reg *plugin.Reg[Settings]) {
		reg.DefaultOn()
		reg.OnProcess(plugin.PhaseLate, func(_ Settings, h *plugin.Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
			synth, err := docs.ScanByAccount(h.Root, h.Ledger)
			if err != nil {
				slog.Warn("docs scan failed", "ledger", h.Ledger, "err", err)
			} else if len(synth) > 0 {
				h.Documents = append(h.Documents, synth...)
			}
			return in
		})
	})
}
