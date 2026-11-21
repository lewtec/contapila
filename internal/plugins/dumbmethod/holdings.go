package dumbmethod

import (
	"errors"
	"fmt"
	"math/big"
	"time"

	"cuelang.org/go/cue"

	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/prices"
)

// rawHolding is one priced asset line from the ledger snapshot.
type rawHolding struct {
	Index     int
	Ledger    string
	Account   string
	Commodity string
	Units     *big.Rat
	Value     *big.Rat // operating currency
	Price     *big.Rat // operating currency per unit; nil when unpriced
	AsOf      string
	Unpriced  bool
	Precision int
}

// loadHoldings snapshots Assets:* holdings in the ledger operating currency.
func loadHoldings(l *engine.Ledger, pdb *prices.DB, asOf time.Time, cfg cue.Value) ([]rawHolding, []string, error) {
	if l == nil {
		return nil, nil, errors.New("no ledger")
	}
	if l.OpCurrency == "" {
		return nil, nil, engine.ErrOpCurrencyUnknown
	}
	lines, _, err := l.NetWorth(asOf)
	if err != nil {
		return nil, nil, err
	}

	out := make([]rawHolding, 0, len(lines))
	var warns []string
	for _, ln := range lines {
		if !booking.IsAsset(ln.Account) {
			continue
		}
		if ln.Units == nil || ln.Units.Sign() == 0 {
			continue
		}
		value := ln.Value
		if value == nil {
			value = new(big.Rat)
		}
		if value.Sign() < 0 {
			warns = append(warns, fmt.Sprintf("%s %s has negative value; ignored", ln.Account, ln.Commodity))
			continue
		}
		price, priced := priceOf(l, pdb, ln.Commodity, asOf, ln.Units, value)
		switch {
		case !priced:
			warns = append(warns, fmt.Sprintf("%s %s has no price; excluded from the plan", ln.Account, ln.Commodity))
		case value.Sign() == 0:
			warns = append(warns, fmt.Sprintf("%s %s has zero market value; excluded from the plan", ln.Account, ln.Commodity))
		}
		out = append(out, rawHolding{
			Index:     len(out),
			Ledger:    l.Name,
			Account:   ln.Account,
			Commodity: ln.Commodity,
			Units:     ln.Units,
			Value:     value,
			Price:     price,
			AsOf:      asOfLabel(asOf),
			Unpriced:  ln.Unpriced || !priced,
			Precision: config.CommodityPolicyFor(cfg, ln.Commodity).Precision,
		})
	}
	return out, warns, nil
}

// priceOf returns the quote price per unit in the operating currency.
func priceOf(l *engine.Ledger, pdb *prices.DB, commodity string, asOf time.Time, units, value *big.Rat) (*big.Rat, bool) {
	if commodity == l.OpCurrency {
		return big.NewRat(1, 1), true
	}
	if pdb != nil {
		if r, _, ok := pdb.Rate(commodity, l.OpCurrency, asOf); ok && r != nil && r.Sign() > 0 {
			return r, true
		}
	}
	if units != nil && units.Sign() > 0 && value != nil && value.Sign() > 0 {
		return new(big.Rat).Quo(value, units), true
	}
	return nil, false
}

func asOfLabel(t time.Time) string {
	if t.IsZero() || !t.Before(engine.AsOfLatest) {
		return "latest"
	}
	return t.Format("2006-01-02")
}
