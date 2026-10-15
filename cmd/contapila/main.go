package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"math/big"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/lewtec/lewkit/x/cmd"
	_ "github.com/lewtec/lewkit/x/driver/prelude" // registers host drivers, including the folder dialog
	"github.com/lewtec/lewkit/x/driver/thread"
	"github.com/lewtec/lewkit/x/entry"
	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/ingest"
	"github.com/lucasew/contapila-go/internal/lsp"
	"github.com/lucasew/contapila-go/internal/parser"
	"github.com/lucasew/contapila-go/internal/period"
	"github.com/lucasew/contapila-go/internal/web"
	"github.com/lucasew/contapila-go/pkg/project"

	// First-party web pages (stream expanders are called from engine, not registered).
	_ "github.com/lucasew/contapila-go/internal/plugins/accountslist"
	_ "github.com/lucasew/contapila-go/internal/plugins/events"
	_ "github.com/lucasew/contapila-go/internal/plugins/queries"
)

// CLI sentinel errors (wrap with context via fmt.Errorf %w).
var (
	ErrZeroLedgers        = errors.New("zero ledgers found")
	ErrLedgersFailed      = errors.New("one or more ledgers failed")
	ErrCheckFailed        = errors.New("check failed")
	ErrTimeFlagsExclusive = errors.New("use either --time or --from/--to, not both")
	ErrFileRequired       = errors.New("--file is required")
	ErrLedgerConflict     = errors.New("ledger argument and --ledger differ")
)

func main() {
	// A release stamp with empty argv is the desktop app (lewkit release run).
	// Any argument stays on the CLI.
	if wantWindow(os.Args[1:]) {
		entry.Main(runApp)
		return
	}

	// Not-a-TTY bare launch / project path → desktop (SPEC §3.2.1).
	applyDesktopRewrite()

	// thread.Run keeps the process main thread free for the macOS web view.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	err := thread.Run(ctx, func(ctx context.Context) error {
		return execute(ctx, os.Args[1:])
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// execute parses argv and runs the selected command. Tests call this instead
// of main() so failures return instead of os.Exit.
func execute(ctx context.Context, args []string) error {
	app, err := cmd.Parse[cmd.App[root]](args...)
	if err != nil {
		return err
	}
	return app.Run(ctx)
}

type root struct {
	Directory project.DirArg `short:"C" long:"directory" help:"run as if contapila started in this directory (project discovery)" default:"" env:"CONTAPILA_DIRECTORY" ctx:"directory"`
	Version   *cmd.VersionCmd
	Init      *initCmd
	Status    *statusCmd
	Doctor    *statusCmd `cmd:"doctor"`
	Check     *checkCmd
	Balances  *balancesCmd
	Journal   *journalCmd
	Todo      *todoCmd
	Pnl       *pnlCmd
	Networth  *networthCmd
	Account   *accountCmd
	Parse     *parseCmd
	Ingest    *ingestCmd
	Web       *webCmd
	Build     *buildCmd
	Desktop   *desktopCmd
	Lsp       *lspCmd
	Dump      *dumpCmd
}

func (root) Description() string {
	return "Contapila — Beancount-class ledger in Go"
}

func (r *root) Run(context.Context) error {
	text, err := cmd.Usage[cmd.App[root]]("contapila")
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(os.Stdout, text)
	return err
}

// projectCwd is -C from the App context bag, else the process working directory.
func projectCwd(ctx context.Context) (string, error) {
	if d := cmd.Get[string](ctx, "directory"); d != "" {
		return d, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return cwd, nil
}

func printDiags(w io.Writer, ds diag.List) {
	if len(ds) == 0 {
		return
	}
	fmt.Fprintln(w, ds.Format())
}

func withLedgers(ctx context.Context, names []string, fn func(*engine.Ledger) error) error {
	cwd, err := projectCwd(ctx)
	if err != nil {
		return err
	}
	h, err := engine.Open(ctx, cwd)
	if err != nil {
		return err
	}
	printDiags(os.Stderr, h.Diags)
	if len(names) == 0 {
		names = h.LedgerNames()
		if len(names) == 0 {
			return ErrZeroLedgers
		}
	}
	var failed bool
	for _, name := range names {
		l, err := h.Ledger(ctx, name)
		if err != nil {
			return err
		}
		if err := fn(l); err != nil {
			failed = true
			fmt.Fprintln(os.Stderr, err)
		}
	}
	if failed {
		return ErrLedgersFailed
	}
	return nil
}

func optionalName(a *engine.LedgerArg) []string {
	if a == nil {
		return nil
	}
	return []string{a.Value()}
}

type timeFlags struct {
	Time cmd.StringArg `long:"time" help:"Fava-style period: 2024, 2024-03, 2024-Q1, month, month-1, year, 2020 - 2024-06" default:""`
	From cmd.StringArg `long:"from" help:"inclusive start YYYY-MM-DD (overrides --time start if set alone with --to)" default:""`
	To   cmd.StringArg `long:"to" help:"inclusive end YYYY-MM-DD" default:""`
}

func (t timeFlags) resolve() (period.Range, error) {
	return resolvePeriod(t.Time.Value(), t.From.Value(), t.To.Value())
}

// resolvePeriod prefers --time; if empty, uses --from/--to; if both empty, all time.
func resolvePeriod(timeFilter, from, to string) (period.Range, error) {
	if timeFilter != "" {
		if from != "" || to != "" {
			return period.Range{}, ErrTimeFlagsExclusive
		}
		return period.Parse(timeFilter, time.Now())
	}
	f, err := engine.ParseDate(from)
	if err != nil {
		return period.Range{}, err
	}
	t, err := engine.ParseDate(to)
	if err != nil {
		return period.Range{}, err
	}
	raw := ""
	if from != "" || to != "" {
		raw = from + " … " + to
	}
	return period.Range{Start: f, End: t, Raw: raw}, nil
}

type statusCmd struct {
}

func (statusCmd) Description() string { return "Show project status" }

func (c *statusCmd) Run(ctx context.Context) error {

	cwd, err := projectCwd(ctx)
	if err != nil {
		return err
	}
	p, err := project.OpenProject(ctx, cwd)
	if err != nil {
		return err
	}
	return writeStatus(os.Stdout, p)
}

func writeStatus(w io.Writer, p *project.Project) error {
	fmt.Fprintf(w, "Project root:      %s\n", p.Root)
	fmt.Fprintf(w, "contapila.cue:     %s\n", filepath.Join(p.Root, "contapila.cue"))
	if len(p.Ledgers) == 0 {
		return ErrZeroLedgers
	}
	fmt.Fprintf(w, "Ledgers (%d):\n", len(p.Ledgers))
	for _, l := range p.Ledgers {
		fmt.Fprintf(w, "  - %s (%s)\n", l.Name, l.MainPath)
	}
	if p.PricesPath != "" {
		switch {
		case p.PricesMissing:
			fmt.Fprintf(w, "Prices:            %s (missing)\n", p.PricesPath)
		case p.PricesEmpty:
			fmt.Fprintf(w, "Prices:            %s (empty)\n", p.PricesPath)
		default:
			fmt.Fprintf(w, "Prices:            %s\n", p.PricesPath)
		}
	}
	if len(p.StreamJournals) > 0 {
		fmt.Fprintf(w, "Stream journals (%d):\n", len(p.StreamJournals))
		for _, j := range p.StreamJournals {
			fmt.Fprintf(w, "  - %s\n", j.Path)
		}
	}
	fmt.Fprintln(w, "CUE:               Unified OK")
	return nil
}

type checkCmd struct {
	Ledger *engine.LedgerArg
}

func (checkCmd) Description() string { return "Validate ledger(s)" }

func (c *checkCmd) Run(ctx context.Context) error {
	return withLedgers(ctx, optionalName(c.Ledger), func(l *engine.Ledger) error {
		fmt.Fprintf(os.Stdout, "== %s ==\n", l.Name)
		ds := l.Check()
		printDiags(os.Stderr, ds)
		if ds.HasErrors() {
			return fmt.Errorf("%w for %s", ErrCheckFailed, l.Name)
		}
		fmt.Fprintln(os.Stdout, "OK")
		return nil
	})
}

// treePadMark is the indent and rollup mark shared by CLI trees.
func treePadMark(depth int, rollup bool) (pad, mark string) {
	pad = strings.Repeat("  ", depth)
	mark = "  "
	if rollup {
		mark = "Σ "
	}
	return pad, mark
}

func printBanner(w io.Writer, title string, r period.Range) {
	fmt.Fprintf(w, "== %s ==", title)
	if !r.Empty() {
		fmt.Fprintf(w, "  [%s]", r.Label())
	}
	fmt.Fprintln(w)
}

func printCommodityAmounts(out io.Writer, m map[string]*big.Rat, empty string) {
	if len(m) == 0 {
		fmt.Fprintln(out, "  "+empty)
		return
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	for _, c := range slices.Sorted(maps.Keys(m)) {
		fmt.Fprintf(w, "  %s\t%s\n", m[c].FloatString(6), c)
	}
	w.Flush()
}

type balancesCmd struct {
	AsOf     cmd.StringArg `long:"as-of" help:"YYYY-MM-DD" default:""`
	Account  []RegexpArg   `long:"account" help:"RE2 pattern, unanchored; repeat to add another"`
	ByLedger ledgerEnum    `long:"ledger" help:"limit to this ledger" default:""`
	Ledger   *engine.LedgerArg
}

func (balancesCmd) Description() string { return "Balances as-of" }

func (c *balancesCmd) ledgerNames() ([]string, error) {
	flag := c.ByLedger.Value()
	pos := optionalName(c.Ledger)
	if flag != "" && len(pos) == 1 && flag != pos[0] {
		return nil, fmt.Errorf("%w: %s and %s", ErrLedgerConflict, pos[0], flag)
	}
	if flag != "" {
		return []string{flag}, nil
	}
	return pos, nil
}

func (c *balancesCmd) Run(ctx context.Context) error {

	t, err := engine.ParseDate(c.AsOf.Value())
	if err != nil {
		return err
	}
	if t.IsZero() {
		t = engine.AsOfLatest
	}
	filters := cmd.Values(c.Account)
	names, err := c.ledgerNames()
	if err != nil {
		return err
	}
	// Single ledger: hierarchical tree. Multi-ledger: flat sorted table.
	if len(names) == 1 {
		return withLedgers(ctx, names, func(l *engine.Ledger) error {
			writeLedgerBalances(os.Stdout, l, t, filters)
			return nil
		})
	}
	type row struct {
		ledger, account, amount, commodity string
	}
	var rows []row
	err = withLedgers(ctx, names, func(l *engine.Ledger) error {
		bals := engine.SelectAccounts(l.BalancesAsOf(t), filters)
		for _, a := range slices.Sorted(maps.Keys(bals)) {
			for _, c := range slices.Sorted(maps.Keys(bals[a])) {
				rows = append(rows, row{
					ledger:    l.Name,
					account:   a,
					amount:    bals[a][c].FloatString(6),
					commodity: c,
				})
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	slices.SortFunc(rows, func(a, b row) int {
		return cmp.Or(
			cmp.Compare(a.account, b.account),
			cmp.Compare(a.commodity, b.commodity),
			cmp.Compare(a.ledger, b.ledger),
		)
	})
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "LEDGER\tACCOUNT\tAMOUNT\tCOMMODITY")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.ledger, r.account, r.amount, r.commodity)
	}
	return w.Flush()
}

func writeLedgerBalances(w io.Writer, l *engine.Ledger, t time.Time, filters []*regexp.Regexp) {
	tree := engine.BalancesTreeFrom(engine.SelectAccounts(l.BalancesAsOf(t), filters))
	fmt.Fprintf(w, "== %s balances ==\n", l.Name)
	for _, ln := range tree {
		pad, mark := treePadMark(ln.Depth, ln.IsRollup)
		name := cmp.Or(ln.Name, ln.Account)
		amt := ""
		if ln.Amount != nil {
			amt = ln.Amount.FloatString(4)
		}
		fmt.Fprintf(w, "%s%s%-28s %12s %s\n", pad, mark, name, amt, ln.Commodity)
	}
}

type journalCmd struct {
	timeFlags
	Ledger *engine.LedgerArg
}

func (journalCmd) Description() string { return "Journal" }

func (c *journalCmd) Run(ctx context.Context) error {

	r, err := c.resolve()
	if err != nil {
		return err
	}
	return withLedgers(ctx, optionalName(c.Ledger), func(l *engine.Ledger) error {
		writeJournal(os.Stdout, l, r)
		return nil
	})
}

func writeJournal(w io.Writer, l *engine.Ledger, r period.Range) {
	printBanner(w, l.Name, r)
	for _, e := range l.Journal(r.Start, r.End) {
		switch e.Kind {
		case "txn":
			fmt.Fprintf(w, "%s * %s\n", e.Date.Format("2006-01-02"), formatPayeeNarration(e.Payee, e.Narration))
			for _, p := range e.Postings {
				if p.Units == nil || p.Units.Commodity == "" && p.Units.Number.Sign() == 0 {
					fmt.Fprintf(w, "  %s\n", p.Account)
					continue
				}
				fmt.Fprintf(w, "  %-40s %s %s\n", p.Account, p.Units.Number.FloatString(4), p.Units.Commodity)
			}
		case "note":
			fmt.Fprintf(w, "%s note %s %q\n", e.Date.Format("2006-01-02"), e.Account, e.Comment)
		case "event":
			fmt.Fprintf(w, "%s event %q %q\n", e.Date.Format("2006-01-02"), e.Narration, e.Comment)
		}
	}
}

type pnlCmd struct {
	timeFlags
	Ledger *engine.LedgerArg
}

func (pnlCmd) Description() string { return "P&L for a Fava-style period" }

func (c *pnlCmd) Run(ctx context.Context) error {

	r, err := c.resolve()
	if err != nil {
		return err
	}
	return withLedgers(ctx, optionalName(c.Ledger), func(l *engine.Ledger) error {
		writePnL(os.Stdout, l, r)
		return nil
	})
}

func writePnL(w io.Writer, l *engine.Ledger, r period.Range) {
	printBanner(w, l.Name, r)
	inc, exp := l.PnLTree(r.Start, r.End)
	printPnLTree := func(title string, lines []engine.PnLLine) {
		fmt.Fprintln(w, title)
		for _, ln := range lines {
			pad, mark := treePadMark(ln.Depth, ln.IsRollup)
			name := cmp.Or(ln.Name, ln.Account)
			fmt.Fprintf(w, "%s%s%-28s %s %s\n", pad, mark, name, ln.Amount.FloatString(2), ln.Commodity)
		}
	}
	printPnLTree("Income:", inc)
	printPnLTree("Expenses:", exp)
}

type networthCmd struct {
	AsOf   cmd.StringArg `long:"as-of" help:"YYYY-MM-DD" default:""`
	Ledger *engine.LedgerArg
}

func (networthCmd) Description() string { return "Net worth" }

func (c *networthCmd) Run(ctx context.Context) error {

	t, err := engine.ParseDate(c.AsOf.Value())
	if err != nil {
		return err
	}
	if t.IsZero() {
		t = engine.AsOfLatest
	}
	return withLedgers(ctx, optionalName(c.Ledger), func(l *engine.Ledger) error {
		return writeNetWorth(os.Stdout, l, t)
	})
}

func writeNetWorth(w io.Writer, l *engine.Ledger, t time.Time) error {
	lines, total, err := l.NetWorthTree(t)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "== %s net worth (%s) ==\n", l.Name, l.OpCurrency)
	for _, ln := range lines {
		pad, mark := treePadMark(ln.Depth, ln.IsRollup)
		name := cmp.Or(ln.Name, ln.Account)
		flag := ""
		if ln.Unpriced {
			flag = " (no px)"
		}
		units := ""
		if ln.Units != nil {
			units = ln.Units.FloatString(4) + " " + ln.Commodity
		}
		fmt.Fprintf(w, "%s%s%-28s %16s => %s %s%s\n",
			pad, mark, name, units, ln.Value.FloatString(2), l.OpCurrency, flag)
	}
	fmt.Fprintf(w, "TOTAL %s %s\n", total.FloatString(2), l.OpCurrency)
	return nil
}

type accountCmd struct {
	timeFlags
	Ledger  engine.LedgerArg
	Account cmd.StringArg
}

func (accountCmd) Description() string {
	return "Show one account (balance, period change, journal)"
}

func (c *accountCmd) Run(ctx context.Context) error {

	if c.Ledger.Value() == "" || c.Account.Value() == "" {
		return fmt.Errorf("%w: account <ledger> <account>", cmd.ErrMissingValue)
	}
	r, err := c.resolve()
	if err != nil {
		return err
	}
	cwd, err := projectCwd(ctx)
	if err != nil {
		return err
	}
	h, err := engine.Open(ctx, cwd)
	if err != nil {
		return err
	}
	printDiags(os.Stderr, h.Diags)
	l, err := c.Ledger.Open(ctx, h)
	if err != nil {
		return err
	}
	writeAccount(os.Stdout, l, c.Account.Value(), r)
	return nil
}

func writeAccount(w io.Writer, l *engine.Ledger, acct string, r period.Range) {
	asOf := r.End
	if asOf.IsZero() {
		asOf = engine.AsOfLatest
	}
	printBanner(w, l.Name+" · "+acct, r)
	fmt.Fprintln(w, "Balance:")
	printCommodityAmounts(w, l.AccountBalances(acct, asOf), "(zero)")
	fmt.Fprintln(w, "Change in period:")
	printCommodityAmounts(w, l.AccountActivity(acct, r.Start, r.End), "(none)")
	fmt.Fprintln(w, "Journal:")
	for _, e := range l.JournalForAccount(acct, r.Start, r.End) {
		if e.Kind != "txn" {
			fmt.Fprintf(w, "%s %s %s\n", e.Date.Format("2006-01-02"), e.Kind, e.Comment)
			continue
		}
		fmt.Fprintf(w, "%s * %s\n", e.Date.Format("2006-01-02"), formatPayeeNarration(e.Payee, e.Narration))
		for _, p := range e.Postings {
			mark := "  "
			if p.Account == acct {
				mark = "* "
			}
			if p.Units == nil {
				fmt.Fprintf(w, "%s%s\n", mark, p.Account)
				continue
			}
			fmt.Fprintf(w, "%s%-40s %s %s\n", mark, p.Account, p.Units.Number.FloatString(4), p.Units.Commodity)
		}
	}
}

// formatPayeeNarration prints Beancount-style "Payee" "Narration" or a single string.
func formatPayeeNarration(payee, narration string) string {
	switch {
	case payee != "" && narration != "":
		return fmt.Sprintf("%q %q", payee, narration)
	case payee != "":
		return fmt.Sprintf("%q", payee)
	default:
		return fmt.Sprintf("%q", narration)
	}
}

type parseCmd struct {
	File cmd.StringArg
}

func (parseCmd) Description() string { return "Dump directives from a file" }

func (c *parseCmd) Run(ctx context.Context) error {

	if c.File.Value() == "" {
		return fmt.Errorf("%w: parse <file>", cmd.ErrMissingValue)
	}
	src, err := os.ReadFile(c.File.Value())
	if err != nil {
		return err
	}
	dirs, diags, err := parser.Parse(c.File.Value(), src)
	printDiags(os.Stderr, diags)
	if err != nil {
		return err
	}
	for _, d := range dirs {
		fmt.Fprintf(os.Stdout, "%T date=%s\n", d, d.GetDate().Format("2006-01-02"))
	}
	return nil
}

// ingestCmd: contapila ingest --file path [-- CMD args…]
// JSONL directives on producer stdout (or contapila stdin if no --).
// With --, contapila stdin is passed through to CMD.
type ingestCmd struct {
	File     cmd.StringArg   `long:"file" help:"target beancount file (created on success if missing)"`
	Producer []cmd.StringArg `help:"producer command; JSONL on its stdout"`
}

func (ingestCmd) Description() string {
	return `Merge JSONL directives into a beancount file

Without extra args, JSONL is read from contapila stdin.
With CMD args (after -- if they look like flags), runs CMD (stdin passed
through) and reads JSONL from CMD stdout. Logs from CMD should go to stderr.

Each JSON line is one directive (full AST-shaped fields). Optional "id" becomes
metadata ingest_id for upsert; without id, lines are appended.
Any error or non-zero CMD exit aborts with no write.`
}

func (c *ingestCmd) Run(ctx context.Context) error {

	if c.File.Value() == "" {
		return ErrFileRequired
	}
	var (
		incoming []ast.Directive
		err      error
	)
	if args := cmd.Values(c.Producer); len(args) > 0 {
		ex := exec.CommandContext(ctx, args[0], args[1:]...)
		ex.Stdin = os.Stdin
		ex.Stderr = os.Stderr
		stdout, errPipe := ex.StdoutPipe()
		if errPipe != nil {
			return errPipe
		}
		if err := ex.Start(); err != nil {
			return err
		}
		incoming, err = ingest.DecodeJSONL(stdout, os.Stderr)
		waitErr := ex.Wait()
		if err != nil {
			return err
		}
		if waitErr != nil {
			return fmt.Errorf("producer failed: %w", waitErr)
		}
	} else {
		incoming, err = ingest.DecodeJSONL(os.Stdin, os.Stderr)
		if err != nil {
			return err
		}
	}

	existing := ""
	if b, rerr := os.ReadFile(c.File.Value()); rerr == nil {
		existing = string(b)
	} else if !errors.Is(rerr, fs.ErrNotExist) {
		return rerr
	}

	out, err := ingest.Apply(existing, c.File.Value(), incoming)
	if err != nil {
		return err
	}
	return ingest.WriteFileAtomic(c.File.Value(), []byte(out))
}

type webCmd struct {
	Addr   cmd.AddrArg `long:"addr" help:"listen address (host:port)" default:"127.0.0.1:8765"`
	Ledger *engine.LedgerArg
}

func (webCmd) Description() string { return "Read-only web UI" }

func (c *webCmd) Run(ctx context.Context) error {

	cwd, err := projectCwd(ctx)
	if err != nil {
		return err
	}
	h, err := engine.Open(ctx, cwd)
	if err != nil {
		return err
	}
	name := ""
	if c.Ledger != nil {
		name = c.Ledger.Value()
	}
	return web.Listen(ctx, h.Project, h.Prices, name, c.Addr.Value())
}

type buildCmd struct {
	Out  cmd.StringArg   `short:"o" long:"out" help:"output directory" default:"site"`
	Jobs cmd.IntArg[int] `short:"j" long:"jobs" help:"parallel render workers (0 = GOMAXPROCS)" default:"0"`
}

func (buildCmd) Description() string {
	return "Write a static HTML site from the project (no time filters)"
}

func (c *buildCmd) Run(ctx context.Context) error {

	cwd, err := projectCwd(ctx)
	if err != nil {
		return err
	}
	if err := web.Build(ctx, cwd, c.Out.Value(), c.Jobs.Value()); err != nil {
		return err
	}
	// Phase detail is on stderr via slog; keep a one-line stdout summary.
	_, err = fmt.Fprintf(os.Stdout, "wrote static site to %s\n", c.Out.Value())
	return err
}

type lspCmd struct {
}

func (lspCmd) Description() string {
	return "Language server (stdio) for Helix and other LSP clients"
}

func (c *lspCmd) Run(ctx context.Context) error {

	// Protocol on stdout; keep slog on stderr.
	return lsp.RunStdio(ctx)
}
