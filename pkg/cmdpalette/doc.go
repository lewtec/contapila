// Package cmdpalette provides a reusable Cmd+K / Ctrl+K jump palette for templ apps.
//
// # Quick start
//
//	import "github.com/lucasew/contapila-go/pkg/cmdpalette"
//
//	p := cmdpalette.New([]cmdpalette.Item{
//		cmdpalette.ItemOf("Reports", "Home", "/"),
//		cmdpalette.ItemOf("Accounts", "Assets:Cash", "/a/cash"),
//	}).WithTitle("Jump").WithButtonClass("btn btn-sm") // optional host classes
//
// In your layout templ:
//
//	@p.Assets() // <head>: CSS + JS (safe every page)
//	@p.Button() // toolbar trigger
//	@p.Modal()  // <dialog> list (end of <body>)
//
// The host only supplies Items (resolved hrefs). Filtering, keyboard open
// (Ctrl/⌘+K), arrows/enter are client-side. Style via CSS variables:
// --cmdpalette-accent, --cmdpalette-bg, --cmdpalette-fg, --cmdpalette-border,
// --cmdpalette-radius.
//
// Contapila wires catalog assembly in internal/web and keeps plugin hooks
// (web.Palette Fill) separate from this UI package.
package cmdpalette
