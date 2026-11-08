package cmdpalette

import (
	_ "embed"
)

//go:embed command.js
var commandJS string

//go:embed cmdpalette.css
var commandCSS string
