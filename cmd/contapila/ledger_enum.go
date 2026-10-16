package main

import (
	"strings"

	"github.com/lewtec/lewkit/x/cmd"
)

// ledgerEnum is --ledger. Parse stores the name.
// balancesCmd checks it against the project once Run has a context.
type ledgerEnum struct {
	name string
}

func (e *ledgerEnum) Parse(s string) error {
	e.name = s
	return nil
}

func (e ledgerEnum) Value() string { return e.name }

// directoryFromArgs returns the last -C / --directory in args.
// The last flag wins, matching lewkit flag parsing.
func directoryFromArgs(args []string) (string, bool) {
	found := ""
	ok := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-C" || arg == "--directory":
			if i+1 >= len(args) {
				return found, ok
			}
			i++
			found = args[i]
			ok = true
		case strings.HasPrefix(arg, "--directory="):
			found = strings.TrimPrefix(arg, "--directory=")
			ok = true
		case strings.HasPrefix(arg, "-C="):
			found = strings.TrimPrefix(arg, "-C=")
			ok = true
		case strings.HasPrefix(arg, "-C") && len(arg) > 2:
			found = arg[2:]
			ok = true
		}
	}
	return found, ok
}

var _ cmd.Parser = (*ledgerEnum)(nil)
