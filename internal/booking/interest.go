package booking

import (
	"math"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/lucasew/contapila-go/internal/ast"
)

// InterestIndicator is the index series name (or FIXED for pure fixed rates).
type InterestIndicator string

const (
	IndicatorFixed InterestIndicator = "FIXED"
	IndicatorCDI   InterestIndicator = "CDI"
	IndicatorIPCA  InterestIndicator = "IPCA"
)

// InterestRate is a parsed interest_rate expression (plugin-compatible).
// Daily growth factor application: bal += bal * (α*idx + PlusDaily).
type InterestRate struct {
	Alpha     *big.Rat // multiplies index daily return (default 1)
	Indicator InterestIndicator
	PlusDaily *big.Rat // fixed daily rate from %aa/%am: (1+r)^(1/n)-1
}

var interestExprRe = regexp.MustCompile(
	`^(?P<rate>[0-9.]+%)?(?P<indicator>CDI|IPCA)?(?P<plus>[+-]?[0-9.]+%(?:a[am])?)?$`,
)

// ParseInterestRate parses expressions like "115% CDI", "IPCA+10%aa", "10% aa".
// Spaces are stripped before matching.
func ParseInterestRate(stmt string) (InterestRate, bool) {
	s := stripSpaces(stmt)
	if s == "" {
		return InterestRate{}, false
	}
	m := interestExprRe.FindStringSubmatch(s)
	if m == nil {
		return InterestRate{}, false
	}
	parsed := map[string]string{}
	for i, name := range interestExprRe.SubexpNames() {
		if i == 0 || name == "" {
			continue
		}
		parsed[name] = m[i]
	}

	// Bare "10%" → fixed plus only (plugin quirk).
	if parsed["rate"] != "" && parsed["indicator"] == "" {
		if parsed["plus"] == "" {
			parsed["plus"] = parsed["rate"]
		}
		parsed["rate"] = ""
	}

	ind := IndicatorFixed
	if parsed["indicator"] != "" {
		ind = InterestIndicator(parsed["indicator"])
	}

	alpha := big.NewRat(1, 1)
	if parsed["rate"] != "" {
		r := strings.TrimSuffix(parsed["rate"], "%")
		v, ok := new(big.Rat).SetString(r)
		if !ok {
			return InterestRate{}, false
		}
		alpha = new(big.Rat).Quo(v, big.NewRat(100, 1))
	}

	plus := big.NewRat(0, 1)
	if parsed["plus"] != "" {
		p, ok := parsePlusDaily(parsed["plus"])
		if !ok {
			return InterestRate{}, false
		}
		plus = p
	}

	return InterestRate{Alpha: alpha, Indicator: ind, PlusDaily: plus}, true
}

func parsePlusDaily(plus string) (*big.Rat, bool) {
	period := "aa"
	body := plus
	if i := strings.Index(body, "%"); i >= 0 {
		tail := body[i+1:]
		body = body[:i]
		if tail == "am" || tail == "aa" {
			period = tail
		}
	}
	body = strings.TrimPrefix(body, "+")
	v, ok := new(big.Rat).SetString(body)
	if !ok {
		return nil, false
	}
	v.Quo(v, big.NewRat(100, 1))
	// Compounding day counts for %aa / %am → daily factor (plugin-compatible).
	const (
		daysPerYearApprox  = 365.0
		daysPerMonthApprox = 30.0
	)
	days := daysPerYearApprox
	if period == "am" {
		days = daysPerMonthApprox
	}
	// (1+r)^(1/days) - 1 — float64 is enough for daily interest factors.
	base, _ := new(big.Rat).Add(big.NewRat(1, 1), v).Float64()
	if base <= 0 {
		return nil, false
	}
	daily := math.Pow(base, 1/days) - 1
	return new(big.Rat).SetFloat64(daily), true
}

func stripSpaces(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// InterestRateFromMeta returns the rate from open metadata (interest_rate or interest-rate).
func InterestRateFromMeta(md ast.Metadata) (InterestRate, string, bool) {
	if len(md) == 0 {
		return InterestRate{}, "", false
	}
	raw, ok := md["interest_rate"]
	if !ok || strings.TrimSpace(raw) == "" {
		raw, ok = md["interest-rate"]
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return InterestRate{}, "", false
	}
	ir, ok := ParseInterestRate(raw)
	return ir, raw, ok
}

// IncomePassivoAccount maps Assets:… → Income:Passivo:… (plugin-compatible).
func IncomePassivoAccount(asset string) string {
	const prefix = "Assets"
	if strings.HasPrefix(asset, prefix) {
		return "Income:Passivo" + asset[len(prefix):]
	}
	return "Income:Passivo:" + asset
}

// IndexDB maps indicator → date (YYYY-MM-DD) → daily return from custom "index".
type IndexDB map[InterestIndicator]map[string]*big.Rat

// LoadIndexDB collects custom "index" rows from the directive stream.
func LoadIndexDB(dirs []ast.Directive) IndexDB {
	db := IndexDB{}
	for _, d := range dirs {
		c, ok := d.(ast.Custom)
		if !ok || c.Type != "index" {
			continue
		}
		ind, rate, ok := parseIndexCustom(c)
		if !ok {
			continue
		}
		if db[ind] == nil {
			db[ind] = map[string]*big.Rat{}
		}
		db[ind][c.Date.Format("2006-01-02")] = new(big.Rat).Set(rate)
	}
	return db
}

func parseIndexCustom(c ast.Custom) (InterestIndicator, *big.Rat, bool) {
	if len(c.Values) < 2 {
		return "", nil, false
	}
	name := strings.TrimSpace(c.Values[0].Text)
	if name == "" {
		return "", nil, false
	}
	var rate *big.Rat
	if c.Values[1].Number != nil {
		rate = new(big.Rat).Set(c.Values[1].Number)
	} else if t := strings.TrimSpace(c.Values[1].Text); t != "" {
		r, ok := new(big.Rat).SetString(t)
		if !ok {
			return "", nil, false
		}
		rate = r
	} else {
		return "", nil, false
	}
	return InterestIndicator(strings.ToUpper(name)), rate, true
}

// IndexRate returns the daily return for indicator on day, or 0 if missing.
func (db IndexDB) IndexRate(ind InterestIndicator, day time.Time) *big.Rat {
	if db == nil || ind == IndicatorFixed {
		return big.NewRat(0, 1)
	}
	m := db[ind]
	if m == nil {
		return big.NewRat(0, 1)
	}
	if r := m[day.Format("2006-01-02")]; r != nil {
		return new(big.Rat).Set(r)
	}
	return big.NewRat(0, 1)
}
