package web

import (
	"bytes"
	"fmt"
	"html"
	"strings"

	"cuelang.org/go/cue"
	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/plugin"
)

func fillDebug(pc PageContext, data *PageData) {
	if pc.Project == nil || pc.Project.Config == nil {
		data.Error = "no project config"
		return
	}
	cfg := pc.Project.Config.Value
	flags, _ := config.PluginFlags(cfg)

	infos := plugin.Infos()
	seen := map[string]bool{}
	for _, info := range infos {
		seen[info.ID] = true
		on := plugin.IsEnabled(cfg, info.ID)
		inCfg := flags != nil && containsPluginFlag(flags, info.ID)
		entry := plugin.PluginValue(cfg, info.ID)
		entryStr := formatCUE(entry)
		if !inCfg {
			if info.DefaultOn {
				entryStr = "(not in unified plugins map — DefaultOn)"
			} else {
				entryStr = "(not in unified plugins map — default off)"
			}
		}
		data.PluginRows = append(data.PluginRows, PluginStatusRow{
			ID:        info.ID,
			Enabled:   on,
			InConfig:  inCfg,
			HasStream: info.HasStream,
			EntryCUE:  highlightCUE(entryStr),
		})
	}
	for id, on := range flags {
		if seen[id] {
			continue
		}
		entry := plugin.PluginValue(cfg, id)
		data.PluginRows = append(data.PluginRows, PluginStatusRow{
			ID:       id,
			Enabled:  on,
			InConfig: true,
			EntryCUE: highlightCUE(formatCUE(entry)),
		})
	}

	pluginsNode := cfg.LookupPath(cue.ParsePath("plugins"))
	data.PluginsCUE = highlightCUE(formatCUE(pluginsNode))
	data.ConfigCUE = highlightCUE(formatCUE(cfg))
}

func containsPluginFlag(flags map[string]bool, id string) bool {
	_, ok := flags[id]
	return ok
}

// formatCUE pretty-prints a unified CUE value as plain source text.
func formatCUE(v cue.Value) string {
	if !v.Exists() {
		return "(absent)"
	}
	s := fmt.Sprintf("%v", v)
	s = strings.TrimSpace(s)
	if s == "" {
		return "(empty)"
	}
	return s
}

// highlightCUE returns HTML (chroma) for CUE source. Safe for templ.Raw:
// chroma escapes content; non-code placeholders are html-escaped.
func highlightCUE(src string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return plainPre("(empty)")
	}
	if strings.HasPrefix(src, "(") && strings.HasSuffix(src, ")") {
		// Placeholders like (absent) / (not in unified …)
		return plainPre(src)
	}
	lexer := lexers.Get("cue")
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	it, err := lexer.Tokenise(nil, src)
	if err != nil {
		return plainPre(src)
	}
	// Classes only — colors from .cue-hl CSS (theme-aware).
	formatter := chromahtml.New(
		chromahtml.WithClasses(true),
		chromahtml.ClassPrefix("cue-"),
		chromahtml.TabWidth(2),
		chromahtml.PreventSurroundingPre(false),
	)
	// style required by API even with classes; "github" is unused for colors.
	style := styles.Get("github")
	if style == nil {
		style = styles.Fallback
	}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, it); err != nil {
		return plainPre(src)
	}
	return `<div class="cue-hl">` + buf.String() + `</div>`
}

func plainPre(s string) string {
	return `<pre class="text-[0.65rem] font-mono whitespace-pre-wrap m-0">` + html.EscapeString(s) + `</pre>`
}
