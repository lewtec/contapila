// Package docsmeta turns document: metadata keys on txns/postings into document directives.
package docsmeta

import (
	"iter"
	"slices"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/plugin"
)

// PluginKey is the journal plugin / CUE plugins map key.
const PluginKey = "docs_meta"

// Settings is plugins.docs_meta.settings (none yet).
type Settings struct{}

func init() {
	plugin.RegisterTyped(PluginKey, func(reg *plugin.Reg[Settings]) {
		reg.DefaultOn()
		reg.OnProcess(plugin.PhaseLate, func(_ Settings, h *plugin.Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
			dirs := slices.Collect(in)
			h.Documents = append(h.Documents, fromMetadata(dirs)...)
			return slices.Values(dirs)
		})
	})
}

// fromMetadata turns document: "path" keys on txns/postings into document directives.
// Account for txn-level document: is the first posting account (if any).
func fromMetadata(dirs []ast.Directive) []ast.Document {
	var out []ast.Document
	for _, d := range dirs {
		t, ok := d.(ast.Transaction)
		if !ok {
			continue
		}
		firstAcct := ""
		if len(t.Postings) > 0 {
			firstAcct = t.Postings[0].Account
		}
		if path := t.Metadata["document"]; path != "" {
			out = append(out, ast.Document{
				Meta:      ast.Meta{Date: t.Date, File: t.File, Line: t.Line},
				Account:   firstAcct,
				Path:      path,
				Synthetic: true,
			})
		}
		for _, p := range t.Postings {
			if path := p.Metadata["document"]; path != "" {
				out = append(out, ast.Document{
					Meta:      ast.Meta{Date: t.Date, File: t.File, Line: t.Line},
					Account:   p.Account,
					Path:      path,
					Synthetic: true,
				})
			}
		}
	}
	return out
}
