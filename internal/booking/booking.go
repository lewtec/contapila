package booking

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
)

// defaultTolerance is the absolute balance/cash residual tolerance used when no
// Engine.Tolerance or per-commodity CommTol is set: half ULP of precision 5 (5e-6).
var defaultTolerance = big.NewRat(5, 1_000_000)

// Position is average-cost inventory for one account+commodity (model A).
type Position struct {
	Units     *big.Rat
	TotalCost *big.Rat // in CostCommodity
	CostComm  string
}

func (p *Position) Avg() *big.Rat {
	if p.Units.Sign() == 0 {
		return big.NewRat(0, 1)
	}
	return new(big.Rat).Quo(new(big.Rat).Set(p.TotalCost), new(big.Rat).Set(p.Units))
}

// Engine books directives and tracks inventory / diagnostics.
type Engine struct {
	// account -> commodity -> position
	Inv map[string]map[string]*Position
	// bare commodity balances without cost (cash etc.) also in Inv with CostComm ""

	Open  map[string]time.Time
	Close map[string]time.Time

	// running balance per account+commodity (units only, for balance assertions)
	Bal map[string]map[string]*big.Rat

	// pad pending: account -> pad directive (last pad wins until used)
	Pad map[string]ast.Pad

	Txns   []BookedTxn
	Notes  []ast.Note
	Events []ast.Event

	Diags diag.List

	// Tolerance is the default absolute tolerance (half ULP of precision 5).
	Tolerance *big.Rat
	// CommTol optional per-commodity absolute tolerance (from CUE/meta policy).
	CommTol map[string]*big.Rat
}

type BookedTxn struct {
	Txn      ast.Transaction
	Postings []FilledPosting
}

type FilledPosting struct {
	Account string
	Units   *ast.Amount // always filled after booking
	// CostBasis is total cost removed/added in cost commodity for inventory moves
	CostBasis *ast.Amount
	Metadata  ast.Metadata // from posting key_value (journal stream; not CUE)
}

func New() *Engine {
	return &Engine{
		Inv:       map[string]map[string]*Position{},
		Open:      map[string]time.Time{},
		Close:     map[string]time.Time{},
		Bal:       map[string]map[string]*big.Rat{},
		Pad:       map[string]ast.Pad{},
		Tolerance: defaultTolerance,
	}
}

// tol returns absolute tolerance for commodity (per-comm if set, else default).
func (e *Engine) tol(comm string) *big.Rat {
	if e.CommTol != nil {
		if t, ok := e.CommTol[comm]; ok && t != nil {
			return t
		}
	}
	if e.Tolerance != nil {
		return e.Tolerance
	}
	return defaultTolerance
}

func (e *Engine) Book(dirs []ast.Directive) {
	// Sort by date, then Beancount-style type rank (open before txn, close last),
	// then source line. Same-day open that appears after a txn in include order
	// must still open the account before the txn is booked.
	indexed := sortedDirectives(dirs)

	for _, d := range indexed {
		switch v := d.(type) {
		case ast.Open:
			e.bookOpen(v)
		case ast.Close:
			e.bookClose(v)
		case ast.Transaction:
			e.bookTxn(v)
		case ast.Balance:
			e.bookBalance(v)
		case ast.Pad:
			e.Pad[v.Account] = v
		case ast.Note:
			e.Notes = append(e.Notes, v)
		case ast.Event:
			e.Events = append(e.Events, v)
		case ast.Option, ast.Commodity, ast.Price, ast.Include, ast.Document, ast.Unknown, ast.Custom:
			// handled elsewhere (Custom index series used by autointerest projection)
		}
	}
}

func sortedDirectives(dirs []ast.Directive) []ast.Directive {
	indexed := make([]ast.Directive, len(dirs))
	copy(indexed, dirs)
	sort.SliceStable(indexed, func(i, j int) bool {
		a, b := indexed[i], indexed[j]
		da, db := a.GetDate(), b.GetDate()
		if !da.Equal(db) {
			return da.Before(db)
		}
		oa, ob := directiveOrder(a), directiveOrder(b)
		if oa != ob {
			return oa < ob
		}
		la, lb := directiveLine(a), directiveLine(b)
		if la != lb && la > 0 && lb > 0 {
			return la < lb
		}
		return false
	})
	return indexed
}

// directiveOrder ranks same-day directives (lower runs first).
// Aligns with Beancount SORT_ORDER (Open before Balance/Txn, Close last),
// with Pad before Balance so same-day pad→balance works under model A.
func directiveOrder(d ast.Directive) int {
	switch d.(type) {
	case ast.Open:
		return -2
	case ast.Pad:
		return -1
	case ast.Balance:
		return 0
	case ast.Document:
		return 2
	case ast.Close:
		return 3
	default:
		// Transaction, Note, Event, Price, Commodity, Option, …
		return 1
	}
}

func directiveLine(d ast.Directive) int {
	type hasLine interface{ GetLine() int }
	if x, ok := d.(hasLine); ok {
		return x.GetLine()
	}
	return 0
}

func (e *Engine) bookOpen(o ast.Open) {
	if prev, ok := e.Open[o.Account]; ok {
		e.Diags.Error(o.File, o.Line, fmt.Sprintf("duplicate open for %s (first %s)", o.Account, prev.Format("2006-01-02")))
		return
	}
	e.Open[o.Account] = o.Date
}

func (e *Engine) bookClose(c ast.Close) {
	e.Close[c.Account] = c.Date
}

func (e *Engine) checkAccount(date time.Time, account, file string, line int) {
	if od, ok := e.Open[account]; !ok {
		e.Diags.Warn(file, line, fmt.Sprintf("account not opened: %s", account))
	} else if date.Before(od) {
		e.Diags.Error(file, line, fmt.Sprintf("transaction before open of %s", account))
	}
	if cd, ok := e.Close[account]; ok && !date.Before(cd) {
		e.Diags.Error(file, line, fmt.Sprintf("posting after close of %s", account))
	}
}

func (e *Engine) bookTxn(t ast.Transaction) {
	// residual index
	resIdx := -1
	weights := map[string]*big.Rat{}
	addW := func(comm string, n *big.Rat) {
		if weights[comm] == nil {
			weights[comm] = big.NewRat(0, 1)
		}
		weights[comm].Add(weights[comm], n)
	}

	type invKey struct{ acct, comm string }
	type invOp struct {
		buy      bool // true = increase, false = reduce
		account  string
		comm     string
		units    *big.Rat // positive magnitude
		total    *big.Rat // cost basis total (positive)
		costComm string
	}

	filled := make([]FilledPosting, len(t.Postings))
	var ops []invOp
	buyNet := map[invKey]*big.Rat{}
	sellNet := map[invKey]*big.Rat{}
	addNet := func(m map[invKey]*big.Rat, k invKey, n *big.Rat) {
		if m[k] == nil {
			m[k] = big.NewRat(0, 1)
		}
		m[k].Add(m[k], n)
	}
	invOpAlready := func(account, comm string, units *big.Rat) bool {
		if units == nil {
			return false
		}
		for _, op := range ops {
			if op.account != account || op.comm != comm {
				continue
			}
			if units.Sign() < 0 && !op.buy {
				if op.units.Cmp(new(big.Rat).Neg(units)) == 0 {
					return true
				}
			}
			if units.Sign() > 0 && op.buy && op.units.Cmp(units) == 0 {
				return true
			}
		}
		return false
	}

	// Phase 1: plan inventory + weights without mutating positions/balances.
	// Oversell is judged on the whole txn net per (account, commodity), not per posting.
	for i, p := range t.Postings {
		e.checkAccount(t.Date, p.Account, t.File, t.Line)
		if p.Units == nil {
			if resIdx >= 0 {
				e.Diags.Error(t.File, t.Line, "multiple residual postings in transaction")
				return
			}
			resIdx = i
			filled[i] = FilledPosting{Account: p.Account, Metadata: p.Metadata}
			continue
		}
		if p.Units.Number == nil {
			e.Diags.Error(t.File, t.Line, fmt.Sprintf("amount missing number on %s", p.Account))
			return
		}
		// Bare number without commodity is not a residual (empty leg is residual).
		if strings.TrimSpace(p.Units.Commodity) == "" {
			e.Diags.Error(t.File, t.Line, fmt.Sprintf("amount missing commodity on %s", p.Account))
			return
		}
		units := new(big.Rat).Set(p.Units.Number)
		comm := p.Units.Commodity
		fp := FilledPosting{
			Account:  p.Account,
			Units:    &ast.Amount{Number: units, Commodity: comm},
			Metadata: p.Metadata,
		}

		// Inventory (model A): explicit cost, @/@@ as cost on increases, or reduce existing position.
		buyUnitCost, buyCostComm, buyCostOK := resolveIncreaseCost(p, units)
		hasInv := p.Cost != nil || e.hasPosition(p.Account, comm) || (units.Sign() > 0 && buyCostOK)
		if p.Cost != nil || (units.Sign() < 0 && e.hasPosition(p.Account, comm)) {
			hasInv = true
		}

		if hasInv && (p.Cost != nil || units.Sign() != 0) {
			k := invKey{p.Account, comm}
			if units.Sign() > 0 {
				// buy: {...} cost, or @/@@ price as cost basis
				unitCost := buyUnitCost
				costComm := buyCostComm
				if !buyCostOK {
					// Increase into an existing lot without braces: book at current average
					// (deposit more USD into a costed FX cash account, residual refund, etc.).
					if pos := e.getPos(p.Account, comm); pos != nil && pos.Units.Sign() > 0 && pos.CostComm != "" {
						unitCost = pos.Avg()
						costComm = pos.CostComm
						buyCostOK = true
					}
				}
				if !buyCostOK {
					e.Diags.Error(t.File, t.Line, fmt.Sprintf("buy of %s %s requires explicit cost {...} or price @/@@", units.FloatString(4), comm))
					return
				}
				total := new(big.Rat).Mul(new(big.Rat).Set(units), new(big.Rat).Set(unitCost))
				ops = append(ops, invOp{
					buy: true, account: p.Account, comm: comm,
					units: new(big.Rat).Set(units), total: total, costComm: costComm,
				})
				addNet(buyNet, k, units)
				fp.CostBasis = &ast.Amount{Number: total, Commodity: costComm}
			} else if units.Sign() < 0 {
				// sell / reduce — cost basis from pre-txn average; oversell checked on nets below
				sellUnits := new(big.Rat).Neg(units) // positive
				addNet(sellNet, k, sellUnits)
				pos := e.getPos(p.Account, comm)
				avg := big.NewRat(0, 1)
				costComm := ""
				if pos != nil && pos.Units.Sign() != 0 {
					avg = pos.Avg()
					costComm = pos.CostComm
				}
				if p.Cost != nil && !p.Cost.Empty && p.Cost.Number != nil && pos != nil && pos.Units.Sign() != 0 {
					// must match average (pre-txn)
					diff := new(big.Rat).Sub(new(big.Rat).Set(p.Cost.Number), avg)
					if diff.Abs(diff).Cmp(e.tol(comm)) > 0 {
						e.Diags.Error(t.File, t.Line, fmt.Sprintf("sell cost %s != average %s", p.Cost.Number.FloatString(6), avg.FloatString(6)))
						return
					}
				}
				totalCost := new(big.Rat).Mul(sellUnits, avg)
				ops = append(ops, invOp{
					buy: false, account: p.Account, comm: comm,
					units: sellUnits, total: totalCost, costComm: costComm,
				})
				fp.CostBasis = &ast.Amount{Number: new(big.Rat).Neg(totalCost), Commodity: costComm}
			}
		}

		filled[i] = fp
	}

	// Face currencies demanded by other legs (stock costs in USD, expenses in USD, …).
	// Spending a foreign-costed pile of that currency weights as face amount for balancing.
	faceDemand := faceCurrencyDemand(t.Postings, filled)

	// Phase 1b: weights for balancing
	for i, p := range t.Postings {
		fp := filled[i]
		if p.Units == nil || fp.Units == nil || fp.Units.Number == nil {
			continue
		}
		units := fp.Units.Number
		comm := fp.Units.Commodity
		w := postingWeight(p, units, fp.CostBasis)
		if cashFaceWeight(p, units, comm, fp.CostBasis, faceDemand) {
			// Face amount in the cash currency (USD), not foreign cost (BRL).
			w = wpair{comm, new(big.Rat).Set(units)}
		}
		addW(w.comm, w.amt)
	}

	// Residual (weights only; balances applied after inventory plan succeeds).
	// One empty posting absorbs -sum(weights) for every residual commodity and
	// expands to one filled amount per commodity on that account.
	if resIdx >= 0 {
		var nonzero []string
		for c, a := range weights {
			if a.Sign() != 0 && new(big.Rat).Abs(a).Cmp(e.tol(c)) > 0 {
				nonzero = append(nonzero, c)
			}
		}
		sort.Strings(nonzero)
		if len(nonzero) == 0 {
			// zero residual — leave units 0 with empty commodity
			filled[resIdx].Units = &ast.Amount{Number: big.NewRat(0, 1), Commodity: ""}
		} else {
			resAcct := filled[resIdx].Account
			resMeta := filled[resIdx].Metadata
			expanded := make([]FilledPosting, 0, len(filled)+len(nonzero)-1)
			for i, fp := range filled {
				if i != resIdx {
					expanded = append(expanded, fp)
					continue
				}
				for _, c := range nonzero {
					n := new(big.Rat).Neg(weights[c])
					expanded = append(expanded, FilledPosting{
						Account:  resAcct,
						Units:    &ast.Amount{Number: n, Commodity: c},
						Metadata: resMeta,
					})
				}
			}
			filled = expanded
		}
		// Residual on a costed inventory account must move inventory (not bare bal only).
		for _, fp := range filled {
			if fp.Units == nil || fp.Units.Number == nil || fp.Units.Number.Sign() == 0 {
				continue
			}
			comm := fp.Units.Commodity
			if !e.hasPosition(fp.Account, comm) {
				continue
			}
			// Skip if already planned from an explicit posting in phase 1.
			if invOpAlready(fp.Account, comm, fp.Units.Number) {
				continue
			}
			pos := e.getPos(fp.Account, comm)
			avg := big.NewRat(0, 1)
			costComm := ""
			if pos != nil && pos.Units.Sign() != 0 {
				avg = pos.Avg()
				costComm = pos.CostComm
			}
			k := invKey{fp.Account, comm}
			if fp.Units.Number.Sign() < 0 {
				sellUnits := new(big.Rat).Neg(fp.Units.Number)
				totalCost := new(big.Rat).Mul(sellUnits, avg)
				ops = append(ops, invOp{
					buy: false, account: fp.Account, comm: comm,
					units: sellUnits, total: totalCost, costComm: costComm,
				})
				addNet(sellNet, k, sellUnits)
			} else if costComm != "" {
				// Residual refund into costed cash: increase at current average.
				total := new(big.Rat).Mul(new(big.Rat).Set(fp.Units.Number), avg)
				ops = append(ops, invOp{
					buy: true, account: fp.Account, comm: comm,
					units: new(big.Rat).Set(fp.Units.Number), total: total, costComm: costComm,
				})
				addNet(buyNet, k, fp.Units.Number)
			}
		}
	} else {
		// must balance
		for c, a := range weights {
			if new(big.Rat).Abs(a).Cmp(e.tol(c)) > 0 {
				e.Diags.Error(t.File, t.Line, fmt.Sprintf("unbalanced transaction for %s: %s", c, a.FloatString(8)))
			}
		}
	}

	// Phase 2: oversell on whole-txn net inventory (start + buys − sells).
	for k, sell := range sellNet {
		if sell == nil || sell.Sign() <= 0 {
			continue
		}
		start := big.NewRat(0, 1)
		if pos := e.getPos(k.acct, k.comm); pos != nil && pos.Units != nil {
			start = new(big.Rat).Set(pos.Units)
		}
		buy := big.NewRat(0, 1)
		if b := buyNet[k]; b != nil {
			buy = b
		}
		available := new(big.Rat).Add(start, buy)
		if sell.Cmp(available) > 0 {
			// warn (not error): do not invent inventory; skip applying this txn's inventory+balances
			if available.Sign() == 0 {
				e.Diags.Warn(t.File, t.Line, fmt.Sprintf("oversell %s: no inventory", k.comm))
			} else {
				e.Diags.Warn(t.File, t.Line, fmt.Sprintf("oversell %s: have %s need %s", k.comm, available.FloatString(6), sell.FloatString(6)))
			}
			return
		}
	}

	// Phase 3: apply inventory — increases first, then reductions (so same-txn buys fund sells).
	for _, op := range ops {
		if !op.buy {
			continue
		}
		if err := e.buy(op.account, op.comm, op.units, op.total, op.costComm); err != nil {
			e.Diags.Error(t.File, t.Line, err.Error())
			return
		}
	}
	for _, op := range ops {
		if op.buy {
			continue
		}
		if err := e.sell(op.account, op.comm, op.units, op.total); err != nil {
			// Should not happen after net oversell check; treat as warn and abort apply.
			e.Diags.Warn(t.File, t.Line, fmt.Sprintf("oversell %s: %v", op.comm, err))
			return
		}
	}

	// Phase 4: bare unit balances for all filled legs
	for _, fp := range filled {
		if fp.Units == nil || fp.Units.Number == nil || fp.Units.Commodity == "" {
			continue
		}
		e.addBal(fp.Account, fp.Units.Commodity, fp.Units.Number)
	}

	e.Txns = append(e.Txns, BookedTxn{Txn: t, Postings: filled})
}

type wpair struct {
	comm string
	amt  *big.Rat
}

// faceCurrencyDemand lists currencies other legs need in face terms: inventory buy
// cost currencies (SPDW @ USD → USD) and bare expense/income/cash weights (19.20 USD).
// Inventory reduces are skipped so a stock sale costed in BRL does not mark BRL as
// "face demand" that would flip an FX cash reduce incorrectly.
func faceCurrencyDemand(postings []ast.Posting, filled []FilledPosting) map[string]bool {
	out := map[string]bool{}
	for i, p := range postings {
		if p.Units == nil {
			continue
		}
		fp := filled[i]
		if fp.Units == nil || fp.Units.Number == nil {
			continue
		}
		units := fp.Units.Number
		// Inventory buy → demand cost currency
		if units.Sign() > 0 && fp.CostBasis != nil && fp.CostBasis.Commodity != "" {
			out[fp.CostBasis.Commodity] = true
			continue
		}
		// Inventory reduce (stock sale / FX lot) — not face demand
		if units.Sign() < 0 && fp.CostBasis != nil {
			continue
		}
		// Expense, income, bare amounts, etc.
		w := postingWeight(p, units, fp.CostBasis)
		if w.comm != "" {
			out[w.comm] = true
		}
	}
	return out
}

// cashFaceWeight is true for FX cash legs (foreign cost basis, no @/{}) when:
//   - reducing to pay for faceDemand legs (expense USD, stocks @ USD), or
//   - increasing (more USD into the lot / residual refund) so residual balances in USD.
//
// Pure FX conversion (USD → BRL + gains) keeps cost-basis weight on reduces when
// faceDemand does not include the unit currency.
func cashFaceWeight(p ast.Posting, units *big.Rat, unitComm string, costBasis *ast.Amount, faceDemand map[string]bool) bool {
	if costBasis == nil || costBasis.Number == nil || costBasis.Commodity == "" {
		return false
	}
	if costBasis.Commodity == unitComm {
		return false // cost already in face currency
	}
	if p.Price != nil {
		return false // priced leg keeps cost/price weight
	}
	if p.Cost != nil && !p.Cost.Empty {
		return false // explicit {...} → cost weight
	}
	if units.Sign() > 0 {
		// Deposit more of a foreign-costed currency without braces.
		return true
	}
	// Spend: only when other legs demand this currency in face terms.
	return faceDemand[unitComm]
}

// resolveIncreaseCost returns unit cost for an inventory increase from {...}
// or, if cost is omitted, from @ (unit) / @@ (total) price.
func resolveIncreaseCost(p ast.Posting, units *big.Rat) (unitCost *big.Rat, costComm string, ok bool) {
	if p.Cost != nil && !p.Cost.Empty && p.Cost.Number != nil {
		return new(big.Rat).Set(p.Cost.Number), p.Cost.Commodity, true
	}
	if p.Price == nil || p.Price.Number == nil || strings.TrimSpace(p.Price.Commodity) == "" {
		return nil, "", false
	}
	if units == nil || units.Sign() == 0 {
		return nil, "", false
	}
	absU := new(big.Rat).Abs(new(big.Rat).Set(units))
	if p.Price.Total {
		// @@ total → unit cost = |total| / |units|
		tot := new(big.Rat).Abs(new(big.Rat).Set(p.Price.Number))
		return new(big.Rat).Quo(tot, absU), p.Price.Commodity, true
	}
	// @ unit price
	return new(big.Rat).Set(p.Price.Number), p.Price.Commodity, true
}

func postingWeight(p ast.Posting, units *big.Rat, costBasis *ast.Amount) wpair {
	// Contapila model A: inventory legs contribute cost basis to the balance so the
	// residual (Income:Gains) absorbs proceeds − cost. Cash legs use explicit amounts.
	// On reductions, @/@@ is proceeds annotation; weight still uses inventory cost basis.
	// On increases without {...}, @/@@ is resolved into CostBasis before this runs.
	if costBasis != nil && costBasis.Number != nil && costBasis.Commodity != "" {
		return wpair{costBasis.Commodity, new(big.Rat).Set(costBasis.Number)}
	}
	if p.Cost != nil && !p.Cost.Empty && p.Cost.Number != nil {
		w := new(big.Rat).Mul(new(big.Rat).Set(units), new(big.Rat).Set(p.Cost.Number))
		return wpair{p.Cost.Commodity, w}
	}
	if p.Price != nil && p.Price.Number != nil {
		var w *big.Rat
		if p.Price.Total {
			w = new(big.Rat).Abs(p.Price.Number)
			if units.Sign() < 0 {
				w.Neg(w)
			}
		} else {
			w = new(big.Rat).Mul(new(big.Rat).Set(units), new(big.Rat).Set(p.Price.Number))
		}
		return wpair{p.Price.Commodity, w}
	}
	return wpair{p.Units.Commodity, new(big.Rat).Set(units)}
}

func (e *Engine) hasPosition(account, comm string) bool {
	m := e.Inv[account]
	if m == nil {
		return false
	}
	p := m[comm]
	return p != nil && p.Units.Sign() != 0
}

func (e *Engine) getPos(account, comm string) *Position {
	m := e.Inv[account]
	if m == nil {
		return nil
	}
	return m[comm]
}

func (e *Engine) buy(account, comm string, units, totalCost *big.Rat, costComm string) error {
	if e.Inv[account] == nil {
		e.Inv[account] = map[string]*Position{}
	}
	pos := e.Inv[account][comm]
	if pos == nil {
		pos = &Position{Units: big.NewRat(0, 1), TotalCost: big.NewRat(0, 1), CostComm: costComm}
		e.Inv[account][comm] = pos
	}
	if pos.CostComm != "" && pos.CostComm != costComm && pos.Units.Sign() != 0 {
		return fmt.Errorf("cost commodity mismatch on %s %s", account, comm)
	}
	pos.CostComm = costComm
	pos.Units.Add(pos.Units, units)
	pos.TotalCost.Add(pos.TotalCost, totalCost)
	return nil
}

func (e *Engine) sell(account, comm string, sellUnits, totalCost *big.Rat) error {
	pos := e.getPos(account, comm)
	if pos == nil {
		return fmt.Errorf("no position")
	}
	pos.Units.Sub(pos.Units, sellUnits)
	pos.TotalCost.Sub(pos.TotalCost, totalCost)
	if pos.Units.Sign() == 0 {
		pos.TotalCost = big.NewRat(0, 1)
	}
	return nil
}

func (e *Engine) addBal(account, comm string, units *big.Rat) {
	if e.Bal[account] == nil {
		e.Bal[account] = map[string]*big.Rat{}
	}
	if e.Bal[account][comm] == nil {
		e.Bal[account][comm] = big.NewRat(0, 1)
	}
	e.Bal[account][comm].Add(e.Bal[account][comm], units)
}

func (e *Engine) balOf(account, comm string) *big.Rat {
	if e.Bal[account] == nil || e.Bal[account][comm] == nil {
		return big.NewRat(0, 1)
	}
	return new(big.Rat).Set(e.Bal[account][comm])
}

func (e *Engine) bookBalance(b ast.Balance) {
	actual := e.balOf(b.Account, b.Amount.Commodity)
	expected := b.Amount.Number
	diff := new(big.Rat).Sub(new(big.Rat).Set(expected), actual)
	tol := e.tol(b.Amount.Commodity)
	if new(big.Rat).Abs(diff).Cmp(tol) <= 0 {
		delete(e.Pad, b.Account)
		return
	}
	// try pad
	if pad, ok := e.Pad[b.Account]; ok {
		// insert balancing: from -> account for diff
		e.checkAccount(b.Date, pad.FromAccount, b.File, b.Line)
		e.addBal(b.Account, b.Amount.Commodity, diff)
		e.addBal(pad.FromAccount, b.Amount.Commodity, new(big.Rat).Neg(diff))
		// also synth txn for journal
		e.Txns = append(e.Txns, BookedTxn{
			Txn: ast.Transaction{
				Meta:      ast.Meta{Date: pad.Date, File: pad.File, Line: pad.Line},
				Flag:      "P",
				Narration: "pad",
			},
			Postings: []FilledPosting{
				{Account: b.Account, Units: &ast.Amount{Number: diff, Commodity: b.Amount.Commodity}},
				{Account: pad.FromAccount, Units: &ast.Amount{Number: new(big.Rat).Neg(diff), Commodity: b.Amount.Commodity}},
			},
		})
		delete(e.Pad, b.Account)
		actual = e.balOf(b.Account, b.Amount.Commodity)
		diff = new(big.Rat).Sub(new(big.Rat).Set(expected), actual)
		if new(big.Rat).Abs(diff).Cmp(tol) <= 0 {
			return
		}
	}
	e.Diags.Error(b.File, b.Line, fmt.Sprintf("balance failed %s: expected %s %s, got %s",
		b.Account, expected.FloatString(6), b.Amount.Commodity, actual.FloatString(6)))
}

// BalancesAsOf returns account+commodity units for txns on or before asOf.
// Note: Engine currently books all at once; for as-of we re-book in report layer if needed.
// Here we expose current Bal after full book.
func (e *Engine) AllBalances() map[string]map[string]*big.Rat {
	out := map[string]map[string]*big.Rat{}
	for acct, m := range e.Bal {
		out[acct] = map[string]*big.Rat{}
		for c, n := range m {
			if n.Sign() != 0 {
				out[acct][c] = new(big.Rat).Set(n)
			}
		}
	}
	return out
}

func IsIncome(account string) bool  { return strings.HasPrefix(account, "Income:") }
func IsExpense(account string) bool { return strings.HasPrefix(account, "Expenses:") }
func IsAsset(account string) bool   { return strings.HasPrefix(account, "Assets:") }
func IsLiability(account string) bool {
	return strings.HasPrefix(account, "Liabilities:")
}
