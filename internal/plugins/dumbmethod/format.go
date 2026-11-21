package dumbmethod

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// money formats an operating-currency amount with two decimals.
func money(r *big.Rat) string {
	if r == nil {
		return ""
	}
	return r.FloatString(2)
}

// percentText formats a percentage value (already multiplied by 100).
func percentText(r *big.Rat) string {
	if r == nil {
		return ""
	}
	return r.FloatString(2) + "%"
}

// unitsText formats a quantity at the commodity precision.
func unitsText(r *big.Rat, precision int) string {
	if r == nil {
		return ""
	}
	if precision < 0 {
		precision = 0
	}
	return r.FloatString(precision)
}

// forceText formats a force score without a trailing ".0" noise.
func forceText(force float64, has bool) string {
	if !has {
		return "—"
	}
	return strconv.FormatFloat(force, 'f', -1, 64)
}

// parseAmount reads a user-entered contribution. It accepts a plain decimal
// number and treats a comma as a decimal separator when no dot is present.
func parseAmount(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return new(big.Rat), nil
	}
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "_", "")
	if !strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", ".")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() < 0 {
		return nil, fmt.Errorf("contribution %q is not a non-negative number", s)
	}
	return r, nil
}
