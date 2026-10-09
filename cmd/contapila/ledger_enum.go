package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lucasew/contapila-go/pkg/project"
)

// ledgerEnum is --ledger. ArgChoices lists project ledgers when help is rendered.
type ledgerEnum struct {
	name string
}

func (e *ledgerEnum) Parse(s string) error {
	if s == "" {
		e.name = ""
		return nil
	}
	names, err := ledgerChoices()
	if err != nil {
		return err
	}
	for _, name := range names {
		if name == s {
			e.name = s
			return nil
		}
	}
	return fmt.Errorf("%w: want one of %s", cmd.ErrInvalidArgument, strings.Join(names, ", "))
}

func (e ledgerEnum) Value() string { return e.name }

// ArgChoices runs when usage text is built, so the set matches the project at --help.
func (ledgerEnum) ArgChoices() []string {
	names, err := ledgerChoices()
	if err != nil {
		return nil
	}
	return names
}

func ledgerChoices() ([]string, error) {
	start, err := ledgerSearchStart(os.Args[1:])
	if err != nil {
		return nil, err
	}
	return project.LedgerNames(start)
}

func ledgerSearchStart(args []string) (string, error) {
	if dir, ok := directoryFromArgs(args); ok {
		return dir, nil
	}
	if dir := os.Getenv("CONTAPILA_DIRECTORY"); dir != "" {
		return dir, nil
	}
	return os.Getwd()
}

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

var (
	_ cmd.Parser     = (*ledgerEnum)(nil)
	_ cmd.ArgChooser = ledgerEnum{}
)
