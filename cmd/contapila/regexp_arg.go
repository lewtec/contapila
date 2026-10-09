package main

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/lewtec/lewkit/x/cmd"
)

// errEmptyPattern rejects "" because regexp.Compile("") matches every string.
var errEmptyPattern = errors.New("empty pattern")

// RegexpArg is a RE2 pattern for an x/cmd flag.
// MatchString is unanchored unless the pattern uses ^ or $.
type RegexpArg struct {
	value *regexp.Regexp
}

func (a *RegexpArg) Parse(arg string) error {
	if arg == "" {
		return fmt.Errorf("%w: %w", cmd.ErrInvalidArgument, errEmptyPattern)
	}
	re, err := regexp.Compile(arg)
	if err != nil {
		return fmt.Errorf("%w: %w", cmd.ErrInvalidArgument, err)
	}
	a.value = re
	return nil
}

func (a RegexpArg) Value() *regexp.Regexp { return a.value }

// MatchString reports whether s matches the pattern.
// A zero RegexpArg does not match.
func (a RegexpArg) MatchString(s string) bool {
	if a.value == nil {
		return false
	}
	return a.value.MatchString(s)
}

var (
	_ cmd.Parser              = (*RegexpArg)(nil)
	_ cmd.Arg[*regexp.Regexp] = (*RegexpArg)(nil)
)
