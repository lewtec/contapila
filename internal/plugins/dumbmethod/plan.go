package dumbmethod

import (
	"fmt"
	"math/big"
	"sort"
)

// ClassInput is one configured asset class and its target share in percent.
type ClassInput struct {
	ID     string
	Target *big.Rat
}

// HoldingInput is one asset holding after CUE classification.
type HoldingInput struct {
	Class   string
	AssetID string
	// AssetLabel is what the report shows for the asset: the user id when set,
	// otherwise the commodity.
	AssetLabel string
	Commodity  string
	Account    string
	Units      *big.Rat
	Value      *big.Rat
	Force      float64
	HasForce   bool
	Price      *big.Rat
	Priced     bool
	Precision  int
}

// ClassPlan is the macro-layer result for one asset class.
type ClassPlan struct {
	ID      string
	Target  *big.Rat // percent
	Current *big.Rat
	Desired *big.Rat
	Gap     *big.Rat
	Budget  *big.Rat
	Spent   *big.Rat
}

// AssetPlan is the micro-layer result for one asset.
type AssetPlan struct {
	Class     string
	ID        string
	Label     string
	Commodity string
	Accounts  []string
	Force     float64
	HasForce  bool
	Current   *big.Rat
	Price     *big.Rat
	Priced    bool
	Desired   *big.Rat
	Gap       *big.Rat
	Buy       *big.Rat
	Units     *big.Rat
	Precision int
	Status    string
}

// Plan is a complete buy-only contribution plan.
type Plan struct {
	Amount *big.Rat
	// Total is the value of classified holdings only (the allocation base).
	Total      *big.Rat
	NewTotal   *big.Rat
	Allocated  *big.Rat
	Uninvested *big.Rat
	// Excluded is the value of holdings with no class (or an unknown class).
	// They are listed for visibility but do not affect the allocation.
	Excluded     *big.Rat
	Classes      []ClassPlan
	Assets       []AssetPlan
	Unclassified []HoldingInput
	Warnings     []string
}

// Compute splits amount across classes and assets using buy-only target
// convergence. Holdings with Value <= 0 are ignored. Holdings without a known
// class are listed in Unclassified and excluded from the allocation base.
func Compute(amount *big.Rat, classes []ClassInput, holdings []HoldingInput) Plan {
	if amount == nil || amount.Sign() < 0 {
		amount = new(big.Rat)
	}
	p := Plan{
		Amount:     clone(amount),
		Total:      new(big.Rat),
		NewTotal:   new(big.Rat),
		Allocated:  new(big.Rat),
		Uninvested: new(big.Rat),
		Excluded:   new(big.Rat),
	}

	classIndex := make(map[string]int, len(classes))
	for _, c := range classes {
		classIndex[c.ID] = len(p.Classes)
		p.Classes = append(p.Classes, ClassPlan{
			ID:      c.ID,
			Target:  clone(c.Target),
			Current: new(big.Rat),
			Desired: new(big.Rat),
			Gap:     new(big.Rat),
			Budget:  new(big.Rat),
			Spent:   new(big.Rat),
		})
	}
	if p.Classes == nil {
		p.Classes = []ClassPlan{}
	}

	type groupKey struct {
		class     string
		id        string
		commodity string
	}
	groups := map[groupKey]*AssetPlan{}
	groupOrder := make([]groupKey, 0, len(holdings))
	seenUnknown := map[string]bool{}
	seenForce := map[string]bool{}

	for _, h := range holdings {
		if h.Value == nil || h.Value.Sign() <= 0 {
			continue
		}

		idx, known := classIndex[h.Class]
		if h.Class == "" {
			p.Unclassified = append(p.Unclassified, h)
			p.Excluded.Add(p.Excluded, h.Value)
			continue
		}
		if !known {
			p.Unclassified = append(p.Unclassified, h)
			p.Excluded.Add(p.Excluded, h.Value)
			if !seenUnknown[h.Class] {
				seenUnknown[h.Class] = true
				p.warnf("holding class %q is not under settings.classes; excluded from the allocation", h.Class)
			}
			continue
		}
		p.Total.Add(p.Total, h.Value)
		p.Classes[idx].Current.Add(p.Classes[idx].Current, h.Value)

		id := h.AssetID
		if id == "" {
			id = h.Account + "|" + h.Commodity
		}
		label := h.AssetLabel
		if label == "" {
			label = h.Commodity
		}
		key := groupKey{class: h.Class, id: id, commodity: h.Commodity}
		ap, ok := groups[key]
		if !ok {
			ap = &AssetPlan{
				Class:     h.Class,
				ID:        id,
				Label:     label,
				Commodity: h.Commodity,
				Current:   new(big.Rat),
				Desired:   new(big.Rat),
				Gap:       new(big.Rat),
				Buy:       new(big.Rat),
			}
			groups[key] = ap
			groupOrder = append(groupOrder, key)
		}
		ap.Current.Add(ap.Current, h.Value)
		ap.Accounts = appendUnique(ap.Accounts, h.Account)
		if h.HasForce {
			if !ap.HasForce {
				ap.Force = h.Force
				ap.HasForce = true
			} else if ap.Force != h.Force {
				forceKey := key.class + "\x1f" + key.id + "\x1f" + key.commodity
				if !seenForce[forceKey] {
					seenForce[forceKey] = true
					p.warnf("asset %q has conflicting force values (%g and %g); using %g", label, ap.Force, h.Force, ap.Force)
				}
			}
		}
		if !ap.Priced && h.Priced && h.Price != nil && h.Price.Sign() > 0 {
			ap.Price = clone(h.Price)
			ap.Priced = true
			ap.Precision = h.Precision
		}
	}

	p.NewTotal.Add(p.Total, p.Amount)

	for i := range p.Classes {
		cp := &p.Classes[i]
		cp.Desired = percentOf(p.NewTotal, cp.Target)
		if cp.Desired.Cmp(cp.Current) > 0 {
			cp.Gap.Sub(cp.Desired, cp.Current)
		}
	}

	var gapSum big.Rat
	for i := range p.Classes {
		gapSum.Add(&gapSum, p.Classes[i].Gap)
	}
	if gapSum.Sign() > 0 {
		scaled := gapSum.Cmp(amount) > 0
		for i := range p.Classes {
			cp := &p.Classes[i]
			if cp.Gap.Sign() <= 0 {
				continue
			}
			if scaled {
				cp.Budget.Quo(new(big.Rat).Mul(amount, cp.Gap), &gapSum)
			} else {
				cp.Budget.Set(cp.Gap)
			}
		}
	}

	for _, key := range groupOrder {
		p.Assets = append(p.Assets, *groups[key])
	}
	sort.SliceStable(p.Assets, func(i, j int) bool {
		ci, cj := classIndex[p.Assets[i].Class], classIndex[p.Assets[j].Class]
		if ci != cj {
			return ci < cj
		}
		if p.Assets[i].Label != p.Assets[j].Label {
			return p.Assets[i].Label < p.Assets[j].Label
		}
		if p.Assets[i].Commodity != p.Assets[j].Commodity {
			return p.Assets[i].Commodity < p.Assets[j].Commodity
		}
		return p.Assets[i].ID < p.Assets[j].ID
	})

	// Micro layer: split each class budget by positive force weights.
	byClass := make(map[string][]int)
	for i := range p.Assets {
		byClass[p.Assets[i].Class] = append(byClass[p.Assets[i].Class], i)
	}
	for i := range p.Classes {
		cp := &p.Classes[i]
		idx := byClass[cp.ID]
		if len(idx) == 0 {
			if cp.Budget.Sign() > 0 {
				p.warnf("class %q has no holdings; %s left uninvested", cp.ID, money(cp.Budget))
			}
			continue
		}

		weightSum := 0.0
		classAfter := new(big.Rat).Add(cp.Current, cp.Budget)
		for _, ai := range idx {
			ap := &p.Assets[ai]
			if !ap.HasForce || ap.Force <= 0 || !ap.Priced || ap.Price == nil || ap.Price.Sign() <= 0 {
				continue
			}
			weightSum += ap.Force
		}
		if weightSum <= 0 {
			if cp.Budget.Sign() > 0 {
				p.warnf("class %q has no buyable asset; %s left uninvested", cp.ID, money(cp.Budget))
			}
			continue
		}

		var assetGapSum big.Rat
		for _, ai := range idx {
			ap := &p.Assets[ai]
			if !ap.HasForce || ap.Force <= 0 || !ap.Priced || ap.Price == nil || ap.Price.Sign() <= 0 {
				continue
			}
			w := new(big.Rat).SetFloat64(ap.Force)
			ap.Desired.Quo(new(big.Rat).Mul(classAfter, w), new(big.Rat).SetFloat64(weightSum))
			if ap.Desired.Cmp(ap.Current) > 0 {
				ap.Gap.Sub(ap.Desired, ap.Current)
				assetGapSum.Add(&assetGapSum, ap.Gap)
			}
		}

		scaledAssetGaps := assetGapSum.Cmp(cp.Budget) > 0
		for _, ai := range idx {
			ap := &p.Assets[ai]
			if ap.Gap.Sign() <= 0 {
				continue
			}
			alloc := new(big.Rat)
			if assetGapSum.Sign() > 0 {
				if scaledAssetGaps {
					alloc.Quo(new(big.Rat).Mul(cp.Budget, ap.Gap), &assetGapSum)
				} else {
					alloc.Set(ap.Gap)
				}
			}
			if alloc.Sign() <= 0 {
				continue
			}
			units := floorToStep(new(big.Rat).Quo(alloc, ap.Price), ap.Precision)
			if units.Sign() <= 0 {
				ap.Status = "budget below one unit"
				continue
			}
			cost := new(big.Rat).Mul(units, ap.Price)
			ap.Units = units
			ap.Buy.Set(cost)
			cp.Spent.Add(cp.Spent, cost)
		}
	}

	for i := range p.Assets {
		ap := &p.Assets[i]
		if ap.Status != "" {
			continue
		}
		switch {
		case !ap.HasForce:
			ap.Status = "no force — no buys"
		case ap.Force <= 0:
			ap.Status = "force 0 — no buys"
		case !ap.Priced || ap.Price == nil || ap.Price.Sign() <= 0:
			ap.Status = "unpriced — no buys"
		case ap.Buy.Sign() > 0:
			ap.Status = ""
		case ap.Gap.Sign() > 0:
			ap.Status = "no budget"
		default:
			ap.Status = "at force-weighted target"
		}
	}

	for i := range p.Assets {
		p.Allocated.Add(p.Allocated, p.Assets[i].Buy)
	}
	p.Uninvested.Sub(p.Amount, p.Allocated)
	if p.Uninvested.Sign() < 0 {
		p.Uninvested.SetInt64(0)
	}

	return p
}

func (p *Plan) warnf(format string, args ...any) {
	p.Warnings = append(p.Warnings, fmt.Sprintf(format, args...))
}

func clone(r *big.Rat) *big.Rat {
	if r == nil {
		return nil
	}
	return new(big.Rat).Set(r)
}

func percentOf(total, pct *big.Rat) *big.Rat {
	if total == nil || pct == nil {
		return new(big.Rat)
	}
	return new(big.Rat).Quo(new(big.Rat).Mul(total, pct), big.NewRat(100, 1))
}

// percent returns 100*part/total, or 0 when total is zero.
func percent(part, total *big.Rat) *big.Rat {
	if part == nil || total == nil || total.Sign() == 0 {
		return new(big.Rat)
	}
	return new(big.Rat).Quo(new(big.Rat).Mul(part, big.NewRat(100, 1)), total)
}

// floorToStep truncates x to a multiple of 10^-precision (toward zero).
func floorToStep(x *big.Rat, precision int) *big.Rat {
	if x == nil {
		return new(big.Rat)
	}
	if precision < 0 {
		precision = 0
	}
	den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(precision)), nil)
	step := new(big.Rat).SetFrac(big.NewInt(1), den)
	q := new(big.Rat).Quo(x, step)
	n := new(big.Int).Quo(q.Num(), q.Denom())
	return new(big.Rat).Mul(new(big.Rat).SetInt(n), step)
}

func appendUnique(list []string, s string) []string {
	if s == "" {
		return list
	}
	for _, v := range list {
		if v == s {
			return list
		}
	}
	return append(list, s)
}
