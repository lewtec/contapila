package config

import (
	"fmt"
	"math/big"
	"strings"

	"cuelang.org/go/cue"
)

// CommodityPolicy is resolved precision/tolerance for one commodity.
type CommodityPolicy struct {
	Precision int
	// Tolerance is the absolute comparison tolerance used by booking.
	Tolerance *big.Rat
}

// DefaultCommodityPolicy is precision 5 → half ULP = 5e-6.
func DefaultCommodityPolicy() CommodityPolicy {
	return CommodityPolicy{
		Precision: 5,
		Tolerance: halfULP(5),
	}
}

// CommodityPolicies builds per-commodity policy from unified CUE config.
// Missing commodities fall back to DefaultCommodityPolicy when looked up via PolicyFor.
func CommodityPolicies(v cue.Value) map[string]CommodityPolicy {
	out := map[string]CommodityPolicy{}
	if !v.Exists() {
		return out
	}
	comms := v.LookupPath(cue.ParsePath("commodities"))
	if !comms.Exists() {
		return out
	}
	iter, err := comms.Fields()
	if err != nil {
		return out
	}
	for iter.Next() {
		name := iter.Selector().String()
		// Strip quotes from CUE label if present.
		name = strings.Trim(name, `"`)
		out[name] = policyFromValue(iter.Value())
	}
	return out
}

// PolicyFor returns policy for commodity, or default if unknown.
func PolicyFor(m map[string]CommodityPolicy, comm string) CommodityPolicy {
	if m != nil {
		if p, ok := m[comm]; ok {
			return p
		}
	}
	return DefaultCommodityPolicy()
}

func policyFromValue(v cue.Value) CommodityPolicy {
	p := DefaultCommodityPolicy()
	if prec := v.LookupPath(cue.ParsePath("precision")); prec.Exists() {
		if n, err := prec.Int64(); err == nil && n >= 0 && n < 32 {
			p.Precision = int(n)
			p.Tolerance = halfULP(p.Precision)
		}
	}
	if tol := v.LookupPath(cue.ParsePath("tolerance")); tol.Exists() {
		if r, ok := cueToRat(tol); ok && r.Sign() >= 0 {
			p.Tolerance = r
		}
	}
	return p
}

// HalfULP returns 0.5 * 10^(-precision) = 5 * 10^(-(precision+1)).
func HalfULP(precision int) *big.Rat {
	if precision < 0 {
		precision = 0
	}
	den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(precision)+1), nil)
	return new(big.Rat).SetFrac(big.NewInt(5), den)
}

func halfULP(precision int) *big.Rat { return HalfULP(precision) }

func cueToRat(v cue.Value) (*big.Rat, bool) {
	if s, err := v.String(); err == nil {
		r := new(big.Rat)
		if _, ok := r.SetString(strings.TrimSpace(s)); ok {
			return r, true
		}
	}
	if n, err := v.Int64(); err == nil {
		return big.NewRat(n, 1), true
	}
	// Prefer JSON/decimal text over float64 (0.01 must stay exact).
	if b, err := v.MarshalJSON(); err == nil {
		s := strings.TrimSpace(string(b))
		s = strings.Trim(s, `"`)
		r := new(big.Rat)
		if _, ok := r.SetString(s); ok {
			return r, true
		}
	}
	if f, err := v.Float64(); err == nil {
		return new(big.Rat).SetFloat64(f), true
	}
	if s := fmt.Sprint(v); s != "" && s != "_" {
		r := new(big.Rat)
		if _, ok := r.SetString(strings.TrimSpace(s)); ok {
			return r, true
		}
	}
	return nil, false
}
