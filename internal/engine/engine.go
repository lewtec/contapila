package engine

import (
	"fmt"
	"log/slog"
	"math/big"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/diag"
	"github.com/lucasew/contapila-go/internal/docs"
	"github.com/lucasew/contapila-go/internal/filesys"
	"github.com/lucasew/contapila-go/internal/loader"
	"github.com/lucasew/contapila-go/internal/period"
	"github.com/lucasew/contapila-go/internal/prices"
	"github.com/lucasew/contapila-go/pkg/project"
)

// AsOfLatest is an as-of far in the future meaning "latest known state".
var AsOfLatest = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

// AccountInfo is an opened account plus metadata from the open directive.
type AccountInfo struct {
	Account    string
	OpenDate   time.Time
	Currencies []string
	Metadata   ast.Metadata
	File       string
	Line       int // 1-based; 0 if unknown
	StartByte  int
	EndByte    int
}

// CommodityInfo is a commodity declaration plus metadata from the commodity directive.
type CommodityInfo struct {
	Currency  string
	Date      time.Time
	Metadata  ast.Metadata
	File      string
	Line      int
	StartByte int
	EndByte   int
}

// Ledger is a fully loaded and booked named ledger.
type Ledger struct {
	Name       string
	Project    *project.Project
	Dirs       []ast.Directive
	Book       *booking.Engine
	Diags      diag.List
	OpCurrency string
	Prices     *prices.DB
	// Documents merges this ledger's `document` directives with <ledger>/docs/by-account.
	Documents []ast.Document
	// Accounts keyed by account name (from open directives, with metadata).
	Accounts map[string]AccountInfo
	// Commodities keyed by currency (from commodity directives in this ledger stream).
	Commodities map[string]CommodityInfo
	// AutoInterest accounts (interest_rate on open) and index series for projection.
	AutoInterest []booking.AutoInterestAccount
	IndexDB      booking.IndexDB
}

// OpenProject wraps project.OpenProject and loads shared prices from disk.
func OpenProject(cwd string) (*project.Project, *prices.DB, diag.List, error) {
	return OpenProjectFS(filesys.OS{}, cwd)
}

// OpenProjectFS opens a project and prices via fsys.
func OpenProjectFS(fsys filesys.FS, cwd string) (*project.Project, *prices.DB, diag.List, error) {
	if fsys == nil {
		fsys = filesys.OS{}
	}
	var diags diag.List
	p, err := project.OpenProjectFS(fsys, cwd)
	if err != nil {
		return nil, nil, diags, err
	}
	db := prices.NewDB()
	if !p.PricesMissing && !p.PricesEmpty {
		pdb, pd, err := prices.LoadFileFS(fsys, p.PricesPath)
		diags.Merge(pd)
		if err != nil {
			slog.Warn("failed loading prices", "err", err)
		} else {
			db = pdb
		}
	}
	return p, db, diags, nil
}

// OpenLedger loads and books one named ledger from disk.
func OpenLedger(p *project.Project, pdb *prices.DB, name string) (*Ledger, error) {
	return OpenLedgerFS(filesys.OS{}, p, pdb, name)
}

// OpenLedgerFS loads and books one named ledger via fsys.
func OpenLedgerFS(fsys filesys.FS, p *project.Project, pdb *prices.DB, name string) (*Ledger, error) {
	if fsys == nil {
		fsys = filesys.OS{}
	}
	var entry string
	for _, l := range p.Ledgers {
		if l.Name == name {
			entry = l.MainPath
			break
		}
	}
	if entry == "" {
		return nil, fmt.Errorf("unknown ledger %q", name)
	}
	dirs, diags, err := loader.LoadFileFS(fsys, entry)
	if err != nil {
		return nil, err
	}
	// filter stream: drop includes
	var stream []ast.Directive
	for _, d := range dirs {
		if _, ok := d.(ast.Include); ok {
			continue
		}
		stream = append(stream, d)
	}
	// Prelude project_journals with role "stream" are auto-injected into every ledger.
	// Skip a path if the ledger stream already loaded that realpath via include.
	stream, idiags := injectProjectStreamJournals(fsys, p, stream)
	diags.Merge(idiags)

	// {cost, date} on postings → synthetic price directives (+ PriceDB points).
	stream = booking.ExpandDatedCosts(stream, pdb)

	// interest_rate opens → income counterpart opens + pad day-before balance.
	diags.Merge(booking.ValidateAutoInterestRates(stream))
	var adiags diag.List
	stream, adiags = booking.ExpandAutoInterest(stream)
	diags.Merge(adiags)

	// Collect documents, opens, commodities after expand (includes synth income opens).
	var ledgerDocs []ast.Document
	accounts := map[string]AccountInfo{}
	commodities := map[string]CommodityInfo{}
	for _, d := range stream {
		switch v := d.(type) {
		case ast.Document:
			ledgerDocs = append(ledgerDocs, v)
		case ast.Open:
			accounts[v.Account] = AccountInfo{
				Account:    v.Account,
				OpenDate:   v.Date,
				Currencies: append([]string(nil), v.Currencies...),
				Metadata:   v.Metadata.Clone(),
				File:       v.File,
				Line:       v.Line,
				StartByte:  v.StartByte,
				EndByte:    v.EndByte,
			}
		case ast.Commodity:
			commodities[v.Currency] = CommodityInfo{
				Currency:  v.Currency,
				Date:      v.Date,
				Metadata:  v.Metadata.Clone(),
				File:      v.File,
				Line:      v.Line,
				StartByte: v.StartByte,
				EndByte:   v.EndByte,
			}
		}
	}

	// CUE ⊔ journal commodity meta → per-commodity booking tolerances.
	commTol := commodityTolerances(p, commodities)
	setupBooking := func(e *booking.Engine) {
		e.CommTol = commTol
	}

	var pdiags diag.List
	stream, pdiags = booking.ExpandPads(stream, setupBooking)
	diags.Merge(pdiags)

	// closing: TRUE expands after residual fill (BookWithClosing), then re-books.
	b, stream, cdiags := booking.BookWithClosing(stream, setupBooking)
	diags.Merge(cdiags)

	autoInterest := booking.CollectAutoInterest(stream)
	indexDB := booking.LoadIndexDB(stream)

	// Expand <ledger>/docs/by-account into synthetic document directives.
	synth, err := docs.ScanByAccount(p.Root, name)
	if err != nil {
		slog.Warn("docs scan failed", "ledger", name, "err", err)
	}
	// Also expand document: "…" keys from txn/posting metadata (runtime only; not CUE).
	fromMeta := docsFromMetadata(stream)
	allDocs := docs.Merge(ledgerDocs, append(synth, fromMeta...))

	op := inferOpCurrency(stream, p)
	return &Ledger{
		Name:         name,
		Project:      p,
		Dirs:         stream,
		Book:         b,
		Diags:        diags,
		OpCurrency:   op,
		Prices:       pdb,
		Documents:    allDocs,
		Accounts:     accounts,
		Commodities:  commodities,
		AutoInterest: autoInterest,
		IndexDB:      indexDB,
	}, nil
}

// injectProjectStreamJournals prepends prelude project_journals (role stream) into the ledger.
// Paths already present in the stream (via include) are skipped to avoid double-load.
func injectProjectStreamJournals(fsys filesys.FS, p *project.Project, stream []ast.Directive) ([]ast.Directive, diag.List) {
	var diags diag.List
	if p == nil || len(p.StreamJournals) == 0 {
		return stream, diags
	}
	if fsys == nil {
		fsys = filesys.OS{}
	}
	present := map[string]bool{}
	for _, d := range stream {
		f := d.GetFile()
		if f == "" {
			continue
		}
		fr, err := filepath.Abs(f)
		if err != nil {
			fr = f
		}
		present[fr] = true
	}
	var prefix []ast.Directive
	for _, j := range p.StreamJournals {
		abs, err := filepath.Abs(j.Path)
		if err != nil {
			abs = j.Path
		}
		if present[abs] {
			continue
		}
		dirs, ldiags, err := loader.LoadFileFS(fsys, j.Path)
		diags.Merge(ldiags)
		if err != nil {
			diags.Error(j.Path, 0, fmt.Sprintf("failed to load project journal %s: %v", j.RelPath, err))
			continue
		}
		for _, d := range dirs {
			if _, ok := d.(ast.Include); ok {
				continue
			}
			prefix = append(prefix, d)
		}
		present[abs] = true
	}
	if len(prefix) == 0 {
		return stream, diags
	}
	out := make([]ast.Directive, 0, len(prefix)+len(stream))
	out = append(out, prefix...)
	out = append(out, stream...)
	return out, diags
}

// commodityTolerances merges CUE #Commodity policy with journal commodity metadata.
// Journal meta keys "precision" / "tolerance" overlay CUE when present.
func commodityTolerances(p *project.Project, journal map[string]CommodityInfo) map[string]*big.Rat {
	out := map[string]*big.Rat{}
	policies := map[string]config.CommodityPolicy{}
	if p != nil && p.Config != nil {
		policies = config.CommodityPolicies(p.Config.Value)
	}
	// Start from CUE.
	for name, pol := range policies {
		if pol.Tolerance != nil {
			out[name] = new(big.Rat).Set(pol.Tolerance)
		}
	}
	// Overlay journal commodity directive metadata.
	for name, info := range journal {
		pol := config.PolicyFor(policies, name)
		if s := strings.TrimSpace(info.Metadata["precision"]); s != "" {
			if n, ok := new(big.Rat).SetString(s); ok && n.IsInt() {
				prec := int(n.Num().Int64())
				if prec >= 0 && prec < 32 {
					pol.Precision = prec
					pol.Tolerance = config.HalfULP(prec)
				}
			}
		}
		if s := strings.TrimSpace(info.Metadata["tolerance"]); s != "" {
			if t, ok := new(big.Rat).SetString(s); ok && t.Sign() >= 0 {
				pol.Tolerance = t
			}
		}
		if pol.Tolerance != nil {
			out[name] = new(big.Rat).Set(pol.Tolerance)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// docsFromMetadata turns document: "path" keys on txns/postings into document directives.
// Account for txn-level document: is the first posting account (if any).
func docsFromMetadata(dirs []ast.Directive) []ast.Document {
	var out []ast.Document
	for _, d := range dirs {
		t, ok := d.(ast.Transaction)
		if !ok {
			continue
		}
		firstAcct := ""
		if len(t.Postings) > 0 {
			firstAcct = t.Postings[0].Account
		}
		if path := t.Metadata["document"]; path != "" {
			out = append(out, ast.Document{
				Meta:      ast.Meta{Date: t.Date, File: t.File, Line: t.Line},
				Account:   firstAcct,
				Path:      path,
				Synthetic: true,
			})
		}
		for _, p := range t.Postings {
			if path := p.Metadata["document"]; path != "" {
				out = append(out, ast.Document{
					Meta:      ast.Meta{Date: t.Date, File: t.File, Line: t.Line},
					Account:   p.Account,
					Path:      path,
					Synthetic: true,
				})
			}
		}
	}
	return out
}

// DocumentsForAccount returns documents linked to account (exact name).
func (l *Ledger) DocumentsForAccount(account string) []ast.Document {
	return docs.ForAccount(l.Documents, account)
}

func inferOpCurrency(dirs []ast.Directive, p *project.Project) string {
	// options first
	for _, d := range dirs {
		if o, ok := d.(ast.Option); ok && o.Key == "operating_currency" {
			return o.Value
		}
	}
	// first transaction commodity
	for _, d := range dirs {
		if t, ok := d.(ast.Transaction); ok {
			for _, post := range t.Postings {
				if post.Units != nil && post.Units.Commodity != "" {
					slog.Warn("operating_currency inferred from first transaction", "commodity", post.Units.Commodity)
					return post.Units.Commodity
				}
			}
		}
	}
	return ""
}

// BalancesAsOf recomputes balances using only directives on or before asOf.
func (l *Ledger) BalancesAsOf(asOf time.Time) map[string]map[string]*big.Rat {
	b := booking.New()
	var subset []ast.Directive
	for _, d := range l.Dirs {
		if d.GetDate().IsZero() || !d.GetDate().After(asOf) {
			subset = append(subset, d)
		}
	}
	b.Book(subset)
	return b.AllBalances()
}

type JournalEntry struct {
	Date      time.Time
	Kind      string // txn, note, event, pad
	Payee     string // txn payee (optional; first quoted string when both present)
	Narration string // txn narration, or event type
	Postings  []booking.FilledPosting
	Account   string
	Comment   string
	// Metadata is txn-level key_value (journal stream only — not unified into CUE).
	Metadata ast.Metadata
}

func (l *Ledger) Journal(from, to time.Time) []JournalEntry {
	return l.journalFiltered(from, to, "")
}

// JournalForAccount returns journal entries that touch account (exact or subaccount).
func (l *Ledger) JournalForAccount(account string, from, to time.Time) []JournalEntry {
	return l.journalFiltered(from, to, account)
}

func inRange(d, from, to time.Time) bool {
	if !from.IsZero() && d.Before(from) {
		return false
	}
	if !to.IsZero() && d.After(to) {
		return false
	}
	return true
}

// AccountMatches reports whether acct is account or a subaccount (Assets:Cash matches Assets:Cash:Wallet).
func AccountMatches(acct, account string) bool {
	if account == "" {
		return true
	}
	return acct == account || strings.HasPrefix(acct, account+":")
}

func (l *Ledger) journalFiltered(from, to time.Time, account string) []JournalEntry {
	var out []JournalEntry
	for _, bt := range l.Book.Txns {
		if !inRange(bt.Txn.Date, from, to) {
			continue
		}
		if account != "" {
			touch := false
			for _, p := range bt.Postings {
				if AccountMatches(p.Account, account) {
					touch = true
					break
				}
			}
			if !touch {
				continue
			}
		}
		out = append(out, JournalEntry{
			Date: bt.Txn.Date, Kind: "txn",
			Payee: bt.Txn.Payee, Narration: bt.Txn.Narration,
			Postings: bt.Postings,
			Metadata: bt.Txn.Metadata,
		})
	}
	for _, n := range l.Book.Notes {
		if !inRange(n.Date, from, to) {
			continue
		}
		if account != "" && !AccountMatches(n.Account, account) {
			continue
		}
		out = append(out, JournalEntry{Date: n.Date, Kind: "note", Account: n.Account, Comment: n.Comment})
	}
	for _, e := range l.Book.Events {
		if account != "" {
			continue // events are not account-scoped
		}
		if !inRange(e.Date, from, to) {
			continue
		}
		out = append(out, JournalEntry{Date: e.Date, Kind: "event", Narration: e.Type, Comment: e.Desc})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}

// AccountBalances returns balances for one account (and optional subaccounts rolled separately).
func (l *Ledger) AccountBalances(account string, asOf time.Time) map[string]*big.Rat {
	all := l.BalancesAsOf(asOf)
	out := map[string]*big.Rat{}
	for acct, byComm := range all {
		if !AccountMatches(acct, account) {
			continue
		}
		// only exact account for the summary strip; subaccounts listed separately in tree later
		if acct != account {
			continue
		}
		for c, n := range byComm {
			out[c] = new(big.Rat).Set(n)
		}
	}
	return out
}

// AccountActivity sums postings to account (exact match only) in [from,to].
func (l *Ledger) AccountActivity(account string, from, to time.Time) map[string]*big.Rat {
	out := map[string]*big.Rat{}
	for _, bt := range l.Book.Txns {
		if !inRange(bt.Txn.Date, from, to) {
			continue
		}
		for _, p := range bt.Postings {
			if p.Account != account || p.Units == nil {
				continue
			}
			c := p.Units.Commodity
			if out[c] == nil {
				out[c] = big.NewRat(0, 1)
			}
			out[c].Add(out[c], p.Units.Number)
		}
	}
	return out
}

// CommodityBalances returns non-zero balances of commodity per account as-of.
func (l *Ledger) CommodityBalances(commodity string, asOf time.Time) map[string]*big.Rat {
	all := l.BalancesAsOf(asOf)
	out := map[string]*big.Rat{}
	for acct, byComm := range all {
		if n, ok := byComm[commodity]; ok && n.Sign() != 0 {
			out[acct] = new(big.Rat).Set(n)
		}
	}
	return out
}

// CommodityActivity sums postings in commodity per account in [from,to].
func (l *Ledger) CommodityActivity(commodity string, from, to time.Time) map[string]*big.Rat {
	out := map[string]*big.Rat{}
	for _, bt := range l.Book.Txns {
		if !inRange(bt.Txn.Date, from, to) {
			continue
		}
		for _, p := range bt.Postings {
			if p.Units == nil || p.Units.Commodity != commodity {
				continue
			}
			if out[p.Account] == nil {
				out[p.Account] = big.NewRat(0, 1)
			}
			out[p.Account].Add(out[p.Account], p.Units.Number)
		}
	}
	return out
}

// JournalForCommodity returns journal entries with at least one posting in commodity.
func (l *Ledger) JournalForCommodity(commodity string, from, to time.Time) []JournalEntry {
	var out []JournalEntry
	for _, bt := range l.Book.Txns {
		if !inRange(bt.Txn.Date, from, to) {
			continue
		}
		touch := false
		for _, p := range bt.Postings {
			if p.Units != nil && p.Units.Commodity == commodity {
				touch = true
				break
			}
		}
		if !touch {
			continue
		}
		out = append(out, JournalEntry{
			Date: bt.Txn.Date, Kind: "txn",
			Payee: bt.Txn.Payee, Narration: bt.Txn.Narration,
			Postings: bt.Postings,
			Metadata: bt.Txn.Metadata,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}

// PnL holds income/expense totals keyed by account then commodity.
// Amounts are native units (not converted); never mix commodities in one cell.
type PnL struct {
	Income   map[string]map[string]*big.Rat // account -> commodity -> amount
	Expenses map[string]map[string]*big.Rat
}

func (l *Ledger) PnL(from, to time.Time) PnL {
	res := PnL{
		Income:   map[string]map[string]*big.Rat{},
		Expenses: map[string]map[string]*big.Rat{},
	}
	add := func(m map[string]map[string]*big.Rat, acct, comm string, n *big.Rat) {
		if m[acct] == nil {
			m[acct] = map[string]*big.Rat{}
		}
		if m[acct][comm] == nil {
			m[acct][comm] = big.NewRat(0, 1)
		}
		m[acct][comm].Add(m[acct][comm], n)
	}
	for _, bt := range l.Book.Txns {
		d := bt.Txn.Date
		if !from.IsZero() && d.Before(from) {
			continue
		}
		if !to.IsZero() && d.After(to) {
			continue
		}
		for _, p := range bt.Postings {
			if p.Units == nil {
				continue
			}
			comm := p.Units.Commodity
			if booking.IsIncome(p.Account) {
				add(res.Income, p.Account, comm, p.Units.Number)
			}
			if booking.IsExpense(p.Account) {
				add(res.Expenses, p.Account, comm, p.Units.Number)
			}
		}
	}
	return res
}

type NetWorthLine struct {
	Account   string
	Commodity string
	Units     *big.Rat
	Value     *big.Rat // in op currency
	Unpriced  bool     // true when marketConvert had no price (value is 0)
}

func (l *Ledger) NetWorth(asOf time.Time) ([]NetWorthLine, *big.Rat, error) {
	if l.OpCurrency == "" {
		return nil, nil, fmt.Errorf("operating currency unknown; set option operating_currency")
	}
	bals := l.BalancesAsOf(asOf)

	var lines []NetWorthLine
	total := big.NewRat(0, 1)
	for acct, m := range bals {
		if !booking.IsAsset(acct) && !booking.IsLiability(acct) {
			continue
		}
		for comm, units := range m {
			if units.Sign() == 0 {
				continue
			}
			// Beancount signs: assets usually debit (+), liabilities credit (−).
			// NW = Σ signed market values (no cost-basis fallback).
			val, unpriced := l.marketConvert(comm, units, asOf, true)
			lines = append(lines, NetWorthLine{Account: acct, Commodity: comm, Units: units, Value: val, Unpriced: unpriced})
			total.Add(total, val)
		}
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].Account != lines[j].Account {
			return lines[i].Account < lines[j].Account
		}
		return lines[i].Commodity < lines[j].Commodity
	})
	return lines, total, nil
}

// marketConvert values units of comm into operating currency at market price only
// (direct, inverse, or one intermediate hop via PriceDB). No cost-basis fallback.
// The bool is true when market price was missing (value is 0).
// logUnpriced should be true for table/NetWorth-style reports; false for dense
// series walks (e.g. PnLBars) to avoid log spam.
func (l *Ledger) marketConvert(comm string, units *big.Rat, asOf time.Time, logUnpriced bool) (*big.Rat, bool) {
	if comm == l.OpCurrency || l.OpCurrency == "" {
		return new(big.Rat).Set(units), false
	}
	if l.Prices != nil {
		if rate, _, ok := l.Prices.Rate(comm, l.OpCurrency, asOf); ok {
			return new(big.Rat).Mul(new(big.Rat).Set(units), rate), false
		}
	}
	if logUnpriced {
		slog.Warn("unpriced commodity; valued at 0 (market only)", "commodity", comm, "op", l.OpCurrency, "asOf", asOf.Format("2006-01-02"))
	}
	return big.NewRat(0, 1), true
}

// LedgerNames helper
func LedgerNames(p *project.Project) []string {
	var names []string
	for _, l := range p.Ledgers {
		names = append(names, l.Name)
	}
	sort.Strings(names)
	return names
}

// ParseDate parses a YYYY-MM-DD calendar date in UTC.
// Empty s yields the zero time and a nil error. Kept as a thin wrapper for cmd/web.
func ParseDate(s string) (time.Time, error) {
	return period.ParseDate(s)
}
