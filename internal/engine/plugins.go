package engine

// First-party stream expanders always linked into processes that open ledgers.
// Registration order matters for PhaseBook: pads before any booker that may be
// imported later (e.g. check_closing from cmd or tests).
import (
	_ "github.com/lucasew/contapila-go/internal/plugins/autointerest"
	_ "github.com/lucasew/contapila-go/internal/plugins/datedcosts"
	_ "github.com/lucasew/contapila-go/internal/plugins/docsfolder"
	_ "github.com/lucasew/contapila-go/internal/plugins/docsmeta"
	_ "github.com/lucasew/contapila-go/internal/plugins/pads"
)
