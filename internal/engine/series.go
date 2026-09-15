package engine

import (
	"math/big"
	"sort"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/period"
)

// SeriesPoint is one chart sample in operating currency.
type SeriesPoint struct {
	Date  time.Time
	Value *big.Rat // op currency
}

// BarPoint is one income/expense bin for diverging bar charts.
type BarPoint struct {
	Start   time.Time
	End     time.Time
	Label   string
	Income  *big.Rat // positive, op currency
	Expense *big.Rat // positive magnitude, op currency (chart draws down)
}

// NetWorthSeries returns net worth in op currency after each event that changes
// asset/liability positions or market prices within [from, to]. Empty bounds = open side.
// Always includes a terminal sample at `to` (or today if open) so the last chart point
// matches the table total (same book units + prices as-of that day).
// Single forward booking pass (not rebook-per-point); price days revalue holdings.
// Leading/trailing zero-value samples are dropped (same idea as PnL empty-bin edge trim).
func (l *Ledger) NetWorthSeries(from, to time.Time) ([]SeriesPoint, error) {
	if l.OpCurrency == "" {
		return nil, nil
	}
	pts, err := l.walkBalanceSeries(from, to, true, func(acct string) bool {
		return booking.IsAsset(acct) || booking.IsLiability(acct)
	}, func(b *booking.Engine, asOf time.Time) *big.Rat {
		return l.netWorthFromBook(b, asOf)
	})
	if err != nil {
		return nil, err
	}
	return trimZeroEdgeSeries(pts), nil
}

// AccountSeries returns market value in op currency after each event touching the
// account or a market price day while the account has a non-zero balance.
func (l *Ledger) AccountSeries(account string, from, to time.Time) ([]SeriesPoint, error) {
	if l.OpCurrency == "" || account == "" {
		return nil, nil
	}
	return l.walkBalanceSeries(from, to, true, func(acct string) bool {
		return AccountMatches(acct, account)
	}, func(b *booking.Engine, asOf time.Time) *big.Rat {
		return l.accountValueFromBook(b, account, asOf)
	})
}

func (l *Ledger) walkBalanceSeries(
	from, to time.Time,
	forceTerminal bool,
	touch func(string) bool,
	value func(*booking.Engine, time.Time) *big.Rat,
) ([]SeriesPoint, error) {
	// Group directives by calendar day (stable within day via original order).
	byDay := map[time.Time][]ast.Directive{}
	priceDay := map[time.Time]bool{}
	forceDay := map[time.Time]bool{}
	var order []time.Time
	addDay := func(dt time.Time) {
		if _, ok := byDay[dt]; !ok {
			order = append(order, dt)
			byDay[dt] = nil
		}
	}
	for _, d := range l.Dirs {
		dt := d.GetDate()
		if dt.IsZero() {
			// options etc. — book at start
			dt = time.Time{}
		} else {
			dt = period.DateOnly(dt)
		}
		addDay(dt)
		byDay[dt] = append(byDay[dt], d)
	}
	// Market revaluation days from shared PriceDB (prices may not be in l.Dirs).
	if l.Prices != nil {
		for _, s := range l.Prices.AllSeries() {
			for _, pt := range s.Points {
				if pt.Date.IsZero() {
					continue
				}
				dt := period.DateOnly(pt.Date)
				addDay(dt)
				priceDay[dt] = true
			}
		}
	}
	// Autointerest projection samples: index days + month-ends through today (or close).
	if len(l.AutoInterest) > 0 {
		today := period.DateOnly(time.Now())
		for _, a := range l.AutoInterest {
			end := today
			if !a.CloseDate.IsZero() && a.CloseDate.Before(end) {
				end = period.DateOnly(a.CloseDate).AddDate(0, 0, -1)
			}
			if end.Before(period.DateOnly(a.OpenDate)) {
				continue
			}
			// Month-ends.
			for d := monthEndOnOrAfter(period.DateOnly(a.OpenDate)); !d.After(end); d = monthEndOnOrAfter(d.AddDate(0, 0, 1)) {
				addDay(d)
				priceDay[d] = true // treat as revaluation sample even if book flat
			}
			// Index series days for this indicator.
			if l.IndexDB != nil {
				if m := l.IndexDB[a.Rate.Indicator]; m != nil {
					for dk := range m {
						if dt, err := period.ParseDate(dk); err == nil {
							if !dt.Before(period.DateOnly(a.OpenDate)) && !dt.After(end) {
								addDay(dt)
								priceDay[dt] = true
							}
						}
					}
				}
			}
		}
	}
	// Terminal day: revalue at filter end (or today) so last point matches NW table as-of.
	if forceTerminal {
		term := to
		if term.IsZero() {
			term = time.Now()
		}
		term = period.DateOnly(term)
		if from.IsZero() || !term.Before(period.DateOnly(from)) {
			addDay(term)
			forceDay[term] = true
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		// undated directives (options, …) book first
		if order[i].IsZero() != order[j].IsZero() {
			return order[i].IsZero()
		}
		return order[i].Before(order[j])
	})

	b := booking.New()
	var out []SeriesPoint
	for _, day := range order {
		dirs := byDay[day]
		if len(dirs) > 0 {
			b.Book(dirs)
		}
		if day.IsZero() {
			continue
		}
		if !from.IsZero() && day.Before(period.DateOnly(from)) {
			continue
		}
		if !to.IsZero() && day.After(period.DateOnly(to)) {
			continue
		}
		// emit if this day could change relevant balances (txn, pad, balance+pad)
		// or revalue via market prices while holdings exist
		hit := forceDay[day]
		if !hit {
			for _, d := range dirs {
				switch v := d.(type) {
				case ast.Transaction:
					for _, p := range v.Postings {
						if touch(p.Account) {
							hit = true
							break
						}
					}
				case ast.Pad:
					if touch(v.Account) || touch(v.FromAccount) {
						hit = true
					}
				case ast.Balance:
					// may apply pending pad into the account
					if touch(v.Account) {
						hit = true
					}
				case ast.Price:
					// ledger-stream prices (also covered by PriceDB dates)
					hit = hasTouchBalance(b, touch)
				}
				if hit {
					break
				}
			}
		}
		if !hit && priceDay[day] {
			hit = hasTouchBalance(b, touch)
		}
		if !hit {
			continue
		}
		out = append(out, SeriesPoint{Date: day, Value: value(b, day)})
	}
	return out, nil
}

// hasTouchBalance reports whether any booked account matching touch has non-zero units.
func hasTouchBalance(b *booking.Engine, touch func(string) bool) bool {
	for acct, m := range b.AllBalances() {
		if !touch(acct) {
			continue
		}
		for _, units := range m {
			if units != nil && units.Sign() != 0 {
				return true
			}
		}
	}
	return false
}

// netWorthFromBook matches NetWorth table: book units only (not autointerest projection),
// market prices as-of the sample date.
func (l *Ledger) netWorthFromBook(b *booking.Engine, asOf time.Time) *big.Rat {
	total := big.NewRat(0, 1)
	for acct, m := range b.AllBalances() {
		if !booking.IsAsset(acct) && !booking.IsLiability(acct) {
			continue
		}
		for comm, units := range m {
			if units == nil || units.Sign() == 0 {
				continue
			}
			// Natural Beancount signs (liabilities are typically negative).
			val, _ := l.marketConvert(comm, units, asOf, false)
			total.Add(total, val)
		}
	}
	return total
}

func (l *Ledger) accountValueFromBook(b *booking.Engine, account string, asOf time.Time) *big.Rat {
	total := big.NewRat(0, 1)
	for acct, m := range b.AllBalances() {
		if !AccountMatches(acct, account) {
			continue
		}
		unitsMap := l.unitsForDisplay(b, acct, m, asOf)
		for comm, units := range unitsMap {
			if units.Sign() == 0 {
				continue
			}
			val, _ := l.marketConvert(comm, units, asOf, false)
			total.Add(total, val)
		}
	}
	// Projected-only autointerest under this prefix.
	for _, a := range l.AutoInterest {
		if !AccountMatches(a.Account, account) {
			continue
		}
		if _, ok := b.AllBalances()[a.Account]; ok {
			continue
		}
		unitsMap := l.unitsForDisplay(b, a.Account, nil, asOf)
		for comm, units := range unitsMap {
			if units.Sign() == 0 {
				continue
			}
			val, _ := l.marketConvert(comm, units, asOf, false)
			total.Add(total, val)
		}
	}
	return total
}

// unitsForDisplay returns book units, or autointerest projection when configured.
func (l *Ledger) unitsForDisplay(b *booking.Engine, acct string, book map[string]*big.Rat, asOf time.Time) map[string]*big.Rat {
	if cfg := l.autoInterestOf(acct); cfg != nil {
		return booking.ProjectedUnits(acct, cfg.Rate, cfg.OpenDate, cfg.CloseDate, l.Dirs, l.IndexDB, asOf)
	}
	if book != nil {
		return book
	}
	return map[string]*big.Rat{}
}

func (l *Ledger) autoInterestOf(acct string) *booking.AutoInterestAccount {
	for i := range l.AutoInterest {
		if l.AutoInterest[i].Account == acct {
			return &l.AutoInterest[i]
		}
	}
	return nil
}

func monthEndOnOrAfter(t time.Time) time.Time {
	t = period.DateOnly(t)
	// Last day of t's month.
	end := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	if !end.Before(t) {
		return end
	}
	return time.Date(t.Year(), t.Month()+2, 0, 0, 0, 0, 0, time.UTC)
}

// PnLBars returns diverging bars for bins from the time filter.
// Beancount signs: income credits are negative, expenses debits positive.
// Chart uses magnitudes: Income = −Σ(income), Expense = Σ(expenses) in op currency.
func (l *Ledger) PnLBars(from, to time.Time, kind period.BinKind) []BarPoint {
	if from.IsZero() || to.IsZero() {
		if len(l.Book.Txns) == 0 {
			return nil
		}
		if from.IsZero() {
			from = l.Book.Txns[0].Txn.Date
			for _, bt := range l.Book.Txns {
				if bt.Txn.Date.Before(from) {
					from = bt.Txn.Date
				}
			}
		}
		if to.IsZero() {
			to = l.Book.Txns[0].Txn.Date
			for _, bt := range l.Book.Txns {
				if bt.Txn.Date.After(to) {
					to = bt.Txn.Date
				}
			}
		}
	}
	r := period.Range{Start: from, End: to}
	bins := period.IterateBins(r, kind)
	out := make([]BarPoint, 0, len(bins))
	for _, b := range bins {
		incSigned := big.NewRat(0, 1) // Beancount signed (usually −)
		expSigned := big.NewRat(0, 1) // Beancount signed (usually +)
		for _, bt := range l.Book.Txns {
			d := bt.Txn.Date
			if d.Before(b.Start) || d.After(b.End) {
				continue
			}
			for _, p := range bt.Postings {
				if p.Units == nil || p.Units.Number == nil {
					continue
				}
				val, _ := l.marketConvert(p.Units.Commodity, p.Units.Number, d, false)
				if booking.IsIncome(p.Account) {
					incSigned.Add(incSigned, val)
				}
				if booking.IsExpense(p.Account) {
					expSigned.Add(expSigned, val)
				}
			}
		}
		// Magnitudes for diverging chart (income up, expense down)
		incMag := new(big.Rat).Neg(incSigned)
		expMag := new(big.Rat).Set(expSigned)
		out = append(out, BarPoint{
			Start: b.Start, End: b.End, Label: binLabel(b, kind),
			Income: incMag, Expense: expMag,
		})
	}
	// Drop empty leading/trailing bins (e.g. year filter still in July → no Aug–Dec padding).
	return trimEmptyEdgeBars(out)
}

func barEmpty(b BarPoint) bool {
	return (b.Income == nil || b.Income.Sign() == 0) && (b.Expense == nil || b.Expense.Sign() == 0)
}

// trimEmptyEdgeBars removes zero-flow bins at the start and end of the series.
// Interior empty bins are kept (gaps between active periods).
func trimEmptyEdgeBars(bars []BarPoint) []BarPoint {
	if len(bars) == 0 {
		return bars
	}
	lo, hi := 0, len(bars)-1
	for lo <= hi && barEmpty(bars[lo]) {
		lo++
	}
	for hi >= lo && barEmpty(bars[hi]) {
		hi--
	}
	if lo > hi {
		return nil
	}
	return bars[lo : hi+1]
}

func seriesPointEmpty(p SeriesPoint) bool {
	return p.Value == nil || p.Value.Sign() == 0
}

// trimZeroEdgeSeries drops leading/trailing zero net-worth samples.
// Interior zeros (temporary wipeouts) are kept.
func trimZeroEdgeSeries(pts []SeriesPoint) []SeriesPoint {
	if len(pts) == 0 {
		return pts
	}
	lo, hi := 0, len(pts)-1
	for lo <= hi && seriesPointEmpty(pts[lo]) {
		lo++
	}
	for hi >= lo && seriesPointEmpty(pts[hi]) {
		hi--
	}
	if lo > hi {
		return nil
	}
	return pts[lo : hi+1]
}

func binLabel(b period.Range, kind period.BinKind) string {
	switch kind {
	case period.BinDay:
		return b.Start.Format("2006-01-02")
	case period.BinWeek:
		return b.Start.Format("2006-01-02")
	case period.BinYear:
		return b.Start.Format("2006")
	default:
		return b.Start.Format("2006-01")
	}
}
