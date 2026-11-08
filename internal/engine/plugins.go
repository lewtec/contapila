package engine

import "github.com/lucasew/contapila-go/internal/config"

// Stream module ids (not registered via plugin.RegisterTyped). Known so
// journal plugin "…" can warn on typos instead of silently no-oping.
func init() {
	for _, id := range []string{
		"dated_costs",
		"autointerest",
		"pads",
		"check_closing",
		"docs_folder",
		"docs_meta",
		"web_accounts",
		"web_events",
		"web_queries",
	} {
		config.RegisterKnownPlugin(id)
	}
}
