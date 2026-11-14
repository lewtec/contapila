package dumbmethod

import (
	"errors"
	"fmt"
	"math/big"

	"cuelang.org/go/cue"
)

// ErrNoClassify means the user has not authored settings.#Classify.
var ErrNoClassify = errors.New("settings.#Classify is not defined")

// classification is the user-authored routing result for one holding.
type classification struct {
	Class    string
	Force    float64
	HasForce bool
	ID       string
}

// holdingFact is the host snapshot shape injected into CUE.
type holdingFact struct {
	Index     int      `json:"index"`
	Ledger    string   `json:"ledger"`
	Account   string   `json:"account"`
	Commodity string   `json:"commodity"`
	Units     *big.Rat `json:"units"`
	Value     *big.Rat `json:"value"`
	Price     *big.Rat `json:"price,omitempty"`
	AsOf      string   `json:"as_of"`
	Unpriced  bool     `json:"unpriced"`
}

// classifiedFact is one user-classified row read back from CUE.
type classifiedFact struct {
	Index *int     `json:"index"`
	Class *string  `json:"class"`
	Force *float64 `json:"force"`
	ID    *string  `json:"id"`
}

// classify unifies host holdings with the user's settings.#Classify and reads
// the classified list. Rules that do not match a holding leave it out of the
// result, which the caller treats as unclassified.
func classify(cfg cue.Value, holdings []rawHolding) (map[int]classification, []string, error) {
	sv := settingsValue(cfg)
	if !sv.Exists() {
		return nil, nil, ErrNoClassify
	}
	def := sv.LookupPath(cue.ParsePath("#Classify"))
	if !def.Exists() {
		return nil, nil, ErrNoClassify
	}
	cctx := def.Context()
	if cctx == nil {
		return nil, nil, errors.New("settings.#Classify has no CUE context")
	}

	facts := struct {
		Holdings []holdingFact `json:"holdings"`
	}{Holdings: make([]holdingFact, 0, len(holdings))}
	for _, h := range holdings {
		facts.Holdings = append(facts.Holdings, holdingFact{
			Index:     h.Index,
			Ledger:    h.Ledger,
			Account:   h.Account,
			Commodity: h.Commodity,
			Units:     h.Units,
			Value:     h.Value,
			Price:     h.Price,
			AsOf:      h.AsOf,
			Unpriced:  h.Unpriced,
		})
	}

	inst := def.Unify(cctx.Encode(facts))
	if err := inst.Validate(); err != nil {
		return nil, nil, fmt.Errorf("settings.#Classify: %w", err)
	}
	list := inst.LookupPath(cue.ParsePath("classified"))
	if !list.Exists() {
		return nil, nil, errors.New("settings.#Classify.classified is not defined")
	}
	var rows []classifiedFact
	if err := list.Decode(&rows); err != nil {
		return nil, nil, fmt.Errorf("settings.#Classify.classified: %w", err)
	}

	out := make(map[int]classification, len(rows))
	var warns []string
	for i, row := range rows {
		if row.Index == nil {
			warns = append(warns, fmt.Sprintf("settings.#Classify.classified[%d] has no index; ignored", i))
			continue
		}
		idx := *row.Index
		if idx < 0 || idx >= len(holdings) {
			warns = append(warns, fmt.Sprintf("settings.#Classify.classified[%d] index %d is out of range; ignored", i, idx))
			continue
		}
		c := out[idx]
		if row.Class != nil {
			c.Class = *row.Class
		}
		if row.Force != nil {
			c.Force = *row.Force
			c.HasForce = true
		}
		if row.ID != nil {
			c.ID = *row.ID
		}
		out[idx] = c
	}
	return out, warns, nil
}
