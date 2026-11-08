package engine

import (
	"context"
	"fmt"

	"github.com/lewtec/lewkit/x/cmd"
)

// LedgerArg is a ledger directory name under the opened project.
type LedgerArg struct {
	name string
}

func (a *LedgerArg) Parse(s string) error {
	if s == "" {
		return fmt.Errorf("%w: ledger", cmd.ErrInvalidArgument)
	}
	a.name = s
	return nil
}

func (a LedgerArg) Value() string { return a.name }

func (a LedgerArg) Open(ctx context.Context, h *Handle) (*Ledger, error) {
	return h.Ledger(ctx, a.name)
}
