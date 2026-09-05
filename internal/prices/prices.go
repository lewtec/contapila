package prices

import (
	"math/big"
	"sort"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
	"github.com/lucasew/contapila-go/internal/filesys"
	"github.com/lucasew/contapila-go/internal/loader"
)

// DB maps base commodity -> quote commodity -> sorted price points.
type DB struct {
	// key base|quote
	series map[string][]Point
}

// Point is one price observation (quote per 1 base on Date).
type Point struct {
	Date     time.Time
	Rate     *big.Rat // quote per 1 base
	Metadata ast.Metadata
	File     string // source file when known
}

func NewDB() *DB {
	return &DB{series: map[string][]Point{}}
}

func PairKey(base, quote string) string { return base + "|" + quote }

// LoadFile loads prices.beancount (includes expanded) from disk.
func LoadFile(path string) (*DB, diag.List, error) {
	return LoadFileFS(filesys.OS{}, path)
}

// LoadFileFS loads prices via fsys (includes expanded).
func LoadFileFS(fsys filesys.FS, path string) (*DB, diag.List, error) {
	db := NewDB()
	dirs, diags, err := loader.LoadFileFS(fsys, path)
	if err != nil {
		return db, diags, err
	}
	for _, d := range dirs {
		if p, ok := d.(ast.Price); ok {
			db.AddPrice(p)
		}
	}
	return db, diags, nil
}

// AddPrice records a price directive (last write wins for same base/quote/date).
func (db *DB) AddPrice(p ast.Price) {
	if p.Amount.Number == nil || p.Amount.Commodity == "" || p.Currency == "" {
		return
	}
	db.add(p.Currency, p.Amount.Commodity, p.Date, p.Amount.Number, p.Metadata.Clone(), p.File)
}

// Add records a rate (no metadata).
func (db *DB) Add(base, quote string, date time.Time, rate *big.Rat) {
	db.add(base, quote, date, rate, nil, "")
}

func (db *DB) add(base, quote string, date time.Time, rate *big.Rat, md ast.Metadata, file string) {
	k := PairKey(base, quote)
	// Last write wins for the same (base, quote, date).
	for i := range db.series[k] {
		if db.series[k][i].Date.Equal(date) {
			db.series[k][i].Rate = new(big.Rat).Set(rate)
			db.series[k][i].Metadata = md
			if file != "" {
				db.series[k][i].File = file
			}
			return
		}
	}
	db.series[k] = append(db.series[k], Point{
		Date:     date,
		Rate:     new(big.Rat).Set(rate),
		Metadata: md,
		File:     file,
	})
	sort.Slice(db.series[k], func(i, j int) bool {
		return db.series[k][i].Date.Before(db.series[k][j].Date)
	})
}

// Rate returns quote per 1 base on or before asOf (market price only).
// Lookup order:
//  1. direct pair base→quote
//  2. inverse of quote→base
//  3. one intermediate hop (e.g. SPDW→USD→BRL)
func (db *DB) Rate(base, quote string, asOf time.Time) (*big.Rat, time.Time, bool) {
	if base == quote {
		return big.NewRat(1, 1), asOf, true
	}
	if r, t, ok := db.directOrInverse(base, quote, asOf); ok {
		return r, t, true
	}
	// One intermediate commodity present on either side of any pair.
	for _, mid := range db.commodities() {
		if mid == base || mid == quote {
			continue
		}
		r1, t1, ok1 := db.directOrInverse(base, mid, asOf)
		if !ok1 {
			continue
		}
		r2, t2, ok2 := db.directOrInverse(mid, quote, asOf)
		if !ok2 {
			continue
		}
		t := t1
		if t2.Before(t1) {
			t = t2
		}
		return new(big.Rat).Mul(r1, r2), t, true
	}
	return nil, time.Time{}, false
}

// direct returns quote per 1 base from an explicit price series only.
func (db *DB) direct(base, quote string, asOf time.Time) (*big.Rat, time.Time, bool) {
	pts := db.series[PairKey(base, quote)]
	var best *Point
	for i := range pts {
		if !pts[i].Date.After(asOf) {
			best = &pts[i]
		}
	}
	if best == nil {
		return nil, time.Time{}, false
	}
	return new(big.Rat).Set(best.Rate), best.Date, true
}

func (db *DB) directOrInverse(base, quote string, asOf time.Time) (*big.Rat, time.Time, bool) {
	if r, t, ok := db.direct(base, quote, asOf); ok {
		return r, t, true
	}
	if r, t, ok := db.direct(quote, base, asOf); ok {
		if r.Sign() == 0 {
			return nil, time.Time{}, false
		}
		return new(big.Rat).Inv(r), t, true
	}
	return nil, time.Time{}, false
}

// commodities returns every currency that appears as base or quote in the DB.
func (db *DB) commodities() []string {
	seen := map[string]bool{}
	for k := range db.series {
		b, q, ok := splitKey(k)
		if !ok {
			continue
		}
		seen[b] = true
		seen[q] = true
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Series is one base/quote pair with its points (oldest first).
type Series struct {
	Base, Quote string
	Points      []Point
}

// AllSeries returns all pairs sorted by base then quote.
func (db *DB) AllSeries() []Series {
	var keys []string
	for k := range db.series {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Series, 0, len(keys))
	for _, k := range keys {
		base, quote, ok := splitKey(k)
		if !ok {
			continue
		}
		pts := db.series[k]
		cp := make([]Point, len(pts))
		copy(cp, pts)
		out = append(out, Series{Base: base, Quote: quote, Points: cp})
	}
	return out
}

// SeriesForBase returns all series where base matches, sorted by quote.
func (db *DB) SeriesForBase(base string) []Series {
	var out []Series
	for _, s := range db.AllSeries() {
		if s.Base == base {
			out = append(out, s)
		}
	}
	return out
}

// Pairs returns distinct base|quote keys for CUE inject (stable order).
func (db *DB) Pairs() []struct{ Base, Quote string } {
	var out []struct{ Base, Quote string }
	for _, s := range db.AllSeries() {
		out = append(out, struct{ Base, Quote string }{s.Base, s.Quote})
	}
	return out
}

func splitKey(k string) (base, quote string, ok bool) {
	i := -1
	for j := 0; j < len(k); j++ {
		if k[j] == '|' {
			i = j
			break
		}
	}
	if i <= 0 || i >= len(k)-1 {
		return "", "", false
	}
	return k[:i], k[i+1:], true
}
