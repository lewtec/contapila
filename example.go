// Package example embeds testdata/starter for contapila init.
package example

import "embed"

//go:embed all:testdata/starter
var FS embed.FS
