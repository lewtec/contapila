package web

import (
	"bytes"
	"html"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// highlightSource returns chroma HTML for src in language lang.
// Safe for templ.Raw (chroma escapes; plain fallbacks use html.EscapeString).
//
// lang is a chroma language name, or the alias "bql" → SQL.
// Token classes use prefix "hl-"; wrap with class "code-hl" (see codeHighlightStyles).
func highlightSource(src, lang string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return plainPre("(empty)")
	}
	// CUE debug placeholders like (absent) / (not in unified …)
	if lang == "cue" && strings.HasPrefix(src, "(") && strings.HasSuffix(src, ")") {
		return plainPre(src)
	}
	chromaLang := lang
	if lang == "bql" {
		chromaLang = "sql"
	}
	lexer := lexers.Get(chromaLang)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	it, err := lexer.Tokenise(nil, src)
	if err != nil {
		return plainPre(src)
	}
	formatter := chromahtml.New(
		chromahtml.WithClasses(true),
		chromahtml.ClassPrefix("hl-"),
		chromahtml.TabWidth(2),
		chromahtml.PreventSurroundingPre(false),
	)
	// style required by API even with classes; colors come from .code-hl CSS.
	style := styles.Get("github")
	if style == nil {
		style = styles.Fallback
	}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, it); err != nil {
		return plainPre(src)
	}
	return `<div class="code-hl">` + buf.String() + `</div>`
}

func plainPre(s string) string {
	return `<pre class="text-[0.65rem] font-mono whitespace-pre-wrap m-0">` + html.EscapeString(s) + `</pre>`
}
