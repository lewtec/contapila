// Package checkclosing registers the check_closing first-party module.
//
// When enabled (journal plugin "check_closing" or CUE plugins."check_closing"),
// posting metadata closing: TRUE expands to balance 0 + close next day after
// residual fill (booking.BookWithClosing).
package checkclosing

import "github.com/lucasew/contapila-go/internal/config"

// PluginKey is the journal plugin / CUE plugins map key.
const PluginKey = "check_closing"

func init() {
	config.RegisterKnownPlugin(PluginKey)
}
