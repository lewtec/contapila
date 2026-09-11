package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/ingest"
	"github.com/lucasew/contapila-go/internal/lsp"
	"github.com/lucasew/contapila-go/internal/parser"
	"github.com/lucasew/contapila-go/internal/period"
	"github.com/lucasew/contapila-go/internal/web"
	"github.com/lucasew/contapila-go/pkg/project"
	"github.com/lucasew/contapila-go/pkg/version"

	// First-party web pages (stream expanders are called from engine, not registered).
	_ "github.com/lucasew/contapila-go/internal/plugins/accountslist"
	_ "github.com/lucasew/contapila-go/internal/plugins/events"
	_ "github.com/lucasew/contapila-go/internal/plugins/queries"
)

// workDir is the optional start directory for project discovery (global -C).
// Empty means use the process working directory.
var workDir string

// CLI sentinel errors (wrap with context via fmt.Errorf %w).
var (
	ErrNotDirectory       = errors.New("not a directory")
	ErrZeroLedgers        = errors.New("zero ledgers found")
	ErrLedgersFailed      = errors.New("one or more ledgers failed")
	ErrCheckFailed        = errors.New("check failed")
	ErrTimeFlagsExclusive = errors.New("use either --time or --from/--to, not both")
	ErrFileRequired       = errors.New("--file is required")
)

func main() {
	// Not-a-TTY bare launch / project path → desktop (SPEC §3.2.1).
	applyDesktopRewrite()

	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return execute(ctx, args)
}

// execute parses argv and runs the selected command. Tests call this instead
// of main() so failures return instead of os.Exit.
func execute(ctx context.Context, args []string) error {
	app, err := cmd.Parse[cmd.App[root]](args...)
	if err != nil {
		return err
	}
	if err := applyParsed(&app.Args); err != nil {
		return err
	}
	if !app.Help() && app.WantVersion() {
		_, err := fmt.Fprintln(os.Stdout, version.GetBuildID())
		return err
	}
	return app.Run(ctx)
}

// applyParsed copies parent-level -C / dump --password so nested commands
// still see flags that were written on the parent spec.
func applyParsed(r *root) error {
	if err := applyDirectory(r.Directory.Value()); err != nil {
		return err
	}
	if d := r.Dump; d != nil {
		dumpPassword = d.Password.Value()
		if err := applyDirectory(d.Directory.Value()); err != nil {
			return err
		}
	}
	return nil
}

// dirFlag is -C/--directory. Embedded on the root (flags before the command)
// and on each command (flags after the command). Parse applies workDir
// immediately so a later ledgerArg can discover the project.
type dirFlag struct {
	Directory directoryArg `short:"C" long:"directory" help:"run as if contapila started in this directory (project discovery)"`
}

// directoryArg is -C: resolve and store the project search start directory.
type directoryArg struct {
	path string
}

func (d *directoryArg) Parse(s string) error {
	if err := applyDirectory(s); err != nil {
		return err
	}
	d.path = workDir
	return nil
}

func (d directoryArg) Value() string { return d.path }

// ledgerArg is a ledger directory name. Parse loads the project and checks
// the name exists; Open books that ledger from a handle.
type ledgerArg struct {
	name string
}

func (a *ledgerArg) Parse(s string) error {
	if s == "" {
		return fmt.Errorf("%w: ledger", cmd.ErrInvalidArgument)
	}
	cwd, err := projectCwd()
	if err != nil {
		return err
	}
	p, err := project.OpenProject(context.Background(), cwd)
	if err != nil {
		return err
	}
	if !projectHasLedger(p, s) {
		return fmt.Errorf("%w %q", engine.ErrUnknownLedger, s)
	}
	a.name = s
	return nil
}

func (a ledgerArg) Value() string { return a.name }

func (a ledgerArg) Open(ctx context.Context, h *engine.Handle) (*engine.Ledger, error) {
	return h.Ledger(ctx, a.name)
}

// cmdFlags is the per-command copy of -C and -v (x/cmd does not inherit parent flags).
type cmdFlags struct {
	dirFlag
	Verbose cmd.Count `short:"v" long:"verbose" help:"log verbosity"`
}

func (f cmdFlags) apply() error {
	if n := f.Verbose.Value(); n > 0 {
		level := slog.LevelInfo - slog.Level(4*n)
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	}
	return applyDirectory(f.Directory.Value())
}

type root struct {
	dirFlag
	Status   *statusCmd
	Doctor   *statusCmd `cmd:"doctor"`
	Check    *checkCmd
	Balances *balancesCmd
	Journal  *journalCmd
	Pnl      *pnlCmd
	Networth *networthCmd
	Account  *accountCmd
	Parse    *parseCmd
	Ingest   *ingestCmd
	Web      *webCmd
	Build    *buildCmd
	Desktop  *desktopCmd
	Lsp      *lspCmd
	Dump     *dumpCmd
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

// applyDirectory resolves -C and stores it in workDir. Empty dir is a no-op
// so a later flag or the desktop rewrite can win.
func applyDirectory(dir string) error {
	if dir == "" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("-C %s: %w", dir, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("-C %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("-C %s: %w", dir, ErrNotDirectory)
	}
	workDir = abs
	return nil
}

// projectCwd returns the project search start directory: -C if set, else process CWD.
func projectCwd() (string, error) {
	if workDir != "" {
		return workDir, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return cwd, nil
}

func printDiags(ds diag.List) {
	if len(ds) == 0 {
		return
	}
	fmt.Fprintln(os.Stderr, ds.Format())
}

func withLedgers(ctx context.Context, names []string, fn func(*engine.Ledger) error) error {
	cwd, err := projectCwd()
	if err != nil {
		return err
	}
	h, err := engine.Open(ctx, cwd)
	if err != nil {
		return err
	}
	printDiags(h.Diags)
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

func optionalName(a *ledgerArg) []string {
	if a == nil {
		return nil
	}
	return []string{a.Value()}
}

type timeFlags struct {
	Time cmd.StringArg `long:"time" help:"Fava-style period: 2024, 2024-03, 2024-Q1, month, month-1, year, 2020 - 2024-06"`
	From cmd.StringArg `long:"from" help:"inclusive start YYYY-MM-DD (overrides --time start if set alone with --to)"`
	To   cmd.StringArg `long:"to" help:"inclusive end YYYY-MM-DD"`
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
	cmdFlags
}

func (statusCmd) Description() string { return "Show project status" }

func (c *statusCmd) Run(ctx context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	cwd, err := projectCwd()
	if err != nil {
		return err
	}
	p, err := project.OpenProject(ctx, cwd)
	if err != nil {
		return err
	}
	fmt.Printf("Project root:      %s\n", p.Root)
	fmt.Printf("contapila.cue:     %s\n", filepath.Join(p.Root, "contapila.cue"))
	if len(p.Ledgers) == 0 {
		return ErrZeroLedgers
	}
	fmt.Printf("Ledgers (%d):\n", len(p.Ledgers))
	for _, l := range p.Ledgers {
		fmt.Printf("  - %s (%s)\n", l.Name, l.MainPath)
	}
	if p.PricesPath != "" {
		switch {
		case p.PricesMissing:
			fmt.Printf("Prices:            %s (missing)\n", p.PricesPath)
		case p.PricesEmpty:
			fmt.Printf("Prices:            %s (empty)\n", p.PricesPath)
		default:
			fmt.Printf("Prices:            %s\n", p.PricesPath)
		}
	}
	if len(p.StreamJournals) > 0 {
		fmt.Printf("Stream journals (%d):\n", len(p.StreamJournals))
		for _, j := range p.StreamJournals {
			fmt.Printf("  - %s\n", j.Path)
		}
	}
	fmt.Println("CUE:               Unified OK")
	return nil
}

type checkCmd struct {
	cmdFlags
	Ledger *ledgerArg
}

func (checkCmd) Description() string { return "Validate ledger(s)" }

func (c *checkCmd) Run(ctx context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	return withLedgers(ctx, optionalName(c.Ledger), func(l *engine.Ledger) error {
		fmt.Printf("== %s ==\n", l.Name)
		ds := l.Check()
		printDiags(ds)
		if ds.HasErrors() {
			return fmt.Errorf("%w for %s", ErrCheckFailed, l.Name)
		}
		fmt.Println("OK")
		return nil
	})
}

type balancesCmd struct {
	cmdFlags
	AsOf   cmd.StringArg `long:"as-of" help:"YYYY-MM-DD"`
	Ledger *ledgerArg
}

func (balancesCmd) Description() string { return "Balances as-of" }

func (c *balancesCmd) Run(ctx context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	t, err := engine.ParseDate(c.AsOf.Value())
	if err != nil {
		return err
	}
	if t.IsZero() {
		t = engine.AsOfLatest
	}
	args := optionalName(c.Ledger)
	// Single ledger: hierarchical tree. Multi-ledger: flat sorted table.
	if len(args) == 1 {
		return withLedgers(ctx, args, func(l *engine.Ledger) error {
			tree := l.BalancesTree(t)
			fmt.Printf("== %s balances ==\n", l.Name)
			for _, ln := range tree {
				pad := strings.Repeat("  ", ln.Depth)
				mark := "  "
				if ln.IsRollup {
					mark = "Σ "
				}
				name := cmp.Or(ln.Name, ln.Account)
				amt := ""
				if ln.Amount != nil {
					amt = ln.Amount.FloatString(4)
				}
				fmt.Printf("%s%s%-28s %12s %s\n", pad, mark, name, amt, ln.Commodity)
			}
			return nil
		})
	}
	type row struct {
		ledger, account, amount, commodity string
	}
	var rows []row
	err = withLedgers(ctx, args, func(l *engine.Ledger) error {
		bals := l.BalancesAsOf(t)
		var accts []string
		for a := range bals {
			accts = append(accts, a)
		}
		sort.Strings(accts)
		for _, a := range accts {
			var cs []string
			for c := range bals[a] {
				cs = append(cs, c)
			}
			sort.Strings(cs)
			for _, c := range cs {
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
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].account != rows[j].account {
			return rows[i].account < rows[j].account
		}
		if rows[i].commodity != rows[j].commodity {
			return rows[i].commodity < rows[j].commodity
		}
		return rows[i].ledger < rows[j].ledger
	})
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "LEDGER\tACCOUNT\tAMOUNT\tCOMMODITY")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.ledger, r.account, r.amount, r.commodity)
	}
	return w.Flush()
}

type journalCmd struct {
	cmdFlags
	timeFlags
	Ledger *ledgerArg
}

func (journalCmd) Description() string { return "Journal" }

func (c *journalCmd) Run(ctx context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	r, err := c.resolve()
	if err != nil {
		return err
	}
	return withLedgers(ctx, optionalName(c.Ledger), func(l *engine.Ledger) error {
		fmt.Printf("== %s ==", l.Name)
		if !r.Empty() {
			fmt.Printf("  [%s]", r.Label())
		}
		fmt.Println()
		for _, e := range l.Journal(r.Start, r.End) {
			switch e.Kind {
			case "txn":
				fmt.Printf("%s * %s\n", e.Date.Format("2006-01-02"), formatPayeeNarration(e.Payee, e.Narration))
				for _, p := range e.Postings {
					if p.Units == nil || p.Units.Commodity == "" && p.Units.Number.Sign() == 0 {
						fmt.Printf("  %s\n", p.Account)
						continue
					}
					fmt.Printf("  %-40s %s %s\n", p.Account, p.Units.Number.FloatString(4), p.Units.Commodity)
				}
			case "note":
				fmt.Printf("%s note %s %q\n", e.Date.Format("2006-01-02"), e.Account, e.Comment)
			case "event":
				fmt.Printf("%s event %q %q\n", e.Date.Format("2006-01-02"), e.Narration, e.Comment)
			}
		}
		return nil
	})
}

type pnlCmd struct {
	cmdFlags
	timeFlags
	Ledger *ledgerArg
}

func (pnlCmd) Description() string { return "P&L for a Fava-style period" }

func (c *pnlCmd) Run(ctx context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	r, err := c.resolve()
	if err != nil {
		return err
	}
	return withLedgers(ctx, optionalName(c.Ledger), func(l *engine.Ledger) error {
		fmt.Printf("== %s ==", l.Name)
		if !r.Empty() {
			fmt.Printf("  [%s]", r.Label())
		}
		fmt.Println()
		inc, exp := l.PnLTree(r.Start, r.End)
		printPnLTree := func(title string, lines []engine.PnLLine) {
			fmt.Println(title)
			for _, ln := range lines {
				pad := strings.Repeat("  ", ln.Depth)
				mark := "  "
				if ln.IsRollup {
					mark = "Σ "
				}
				name := cmp.Or(ln.Name, ln.Account)
				fmt.Printf("%s%s%-28s %s %s\n", pad, mark, name, ln.Amount.FloatString(2), ln.Commodity)
			}
		}
		printPnLTree("Income:", inc)
		printPnLTree("Expenses:", exp)
		return nil
	})
}

type networthCmd struct {
	cmdFlags
	AsOf   cmd.StringArg `long:"as-of" help:"YYYY-MM-DD"`
	Ledger *ledgerArg
}

func (networthCmd) Description() string { return "Net worth" }

func (c *networthCmd) Run(ctx context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	t, err := engine.ParseDate(c.AsOf.Value())
	if err != nil {
		return err
	}
	if t.IsZero() {
		t = engine.AsOfLatest
	}
	return withLedgers(ctx, optionalName(c.Ledger), func(l *engine.Ledger) error {
		lines, total, err := l.NetWorthTree(t)
		if err != nil {
			return err
		}
		fmt.Printf("== %s net worth (%s) ==\n", l.Name, l.OpCurrency)
		for _, ln := range lines {
			pad := strings.Repeat("  ", ln.Depth)
			mark := "  "
			if ln.IsRollup {
				mark = "Σ "
			}
			name := cmp.Or(ln.Name, ln.Account)
			flag := ""
			if ln.Unpriced {
				flag = " (no px)"
			}
			units := ""
			if ln.Units != nil {
				units = ln.Units.FloatString(4) + " " + ln.Commodity
			}
			fmt.Printf("%s%s%-28s %16s => %s %s%s\n",
				pad, mark, name, units, ln.Value.FloatString(2), l.OpCurrency, flag)
		}
		fmt.Printf("TOTAL %s %s\n", total.FloatString(2), l.OpCurrency)
		return nil
	})
}

type accountCmd struct {
	cmdFlags
	timeFlags
	Ledger  ledgerArg
	Account cmd.StringArg
}

func (accountCmd) Description() string {
	return "Show one account (balance, period change, journal)"
}

func (c *accountCmd) Run(ctx context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	if c.Ledger.Value() == "" || c.Account.Value() == "" {
		return fmt.Errorf("%w: account <ledger> <account>", cmd.ErrMissingValue)
	}
	r, err := c.resolve()
	if err != nil {
		return err
	}
	cwd, err := projectCwd()
	if err != nil {
		return err
	}
	h, err := engine.Open(ctx, cwd)
	if err != nil {
		return err
	}
	printDiags(h.Diags)
	l, err := c.Ledger.Open(ctx, h)
	if err != nil {
		return err
	}
	acct := c.Account.Value()
	asOf := r.End
	if asOf.IsZero() {
		asOf = engine.AsOfLatest
	}
	fmt.Printf("== %s · %s ==", l.Name, acct)
	if !r.Empty() {
		fmt.Printf("  [%s]", r.Label())
	}
	fmt.Println()
	fmt.Println("Balance:")
	bals := l.AccountBalances(acct, asOf)
	if len(bals) == 0 {
		fmt.Println("  (zero)")
	} else {
		var cs []string
		for c := range bals {
			cs = append(cs, c)
		}
		sort.Strings(cs)
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		for _, c := range cs {
			fmt.Fprintf(w, "  %s\t%s\n", bals[c].FloatString(6), c)
		}
		w.Flush()
	}
	fmt.Println("Change in period:")
	act := l.AccountActivity(acct, r.Start, r.End)
	if len(act) == 0 {
		fmt.Println("  (none)")
	} else {
		var cs []string
		for c := range act {
			cs = append(cs, c)
		}
		sort.Strings(cs)
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		for _, c := range cs {
			fmt.Fprintf(w, "  %s\t%s\n", act[c].FloatString(6), c)
		}
		w.Flush()
	}
	fmt.Println("Journal:")
	for _, e := range l.JournalForAccount(acct, r.Start, r.End) {
		if e.Kind != "txn" {
			fmt.Printf("%s %s %s\n", e.Date.Format("2006-01-02"), e.Kind, e.Comment)
			continue
		}
		fmt.Printf("%s * %s\n", e.Date.Format("2006-01-02"), formatPayeeNarration(e.Payee, e.Narration))
		for _, p := range e.Postings {
			mark := "  "
			if p.Account == acct {
				mark = "* "
			}
			if p.Units == nil {
				fmt.Printf("%s%s\n", mark, p.Account)
				continue
			}
			fmt.Printf("%s%-40s %s %s\n", mark, p.Account, p.Units.Number.FloatString(4), p.Units.Commodity)
		}
	}
	return nil
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
	cmdFlags
	File cmd.StringArg
}

func (parseCmd) Description() string { return "Dump directives from a file" }

func (c *parseCmd) Run(ctx context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	if c.File.Value() == "" {
		return fmt.Errorf("%w: parse <file>", cmd.ErrMissingValue)
	}
	src, err := os.ReadFile(c.File.Value())
	if err != nil {
		return err
	}
	dirs, diags, err := parser.Parse(c.File.Value(), src)
	printDiags(diags)
	if err != nil {
		return err
	}
	for _, d := range dirs {
		fmt.Printf("%T date=%s\n", d, d.GetDate().Format("2006-01-02"))
	}
	return nil
}

// ingestCmd: contapila ingest --file path [-- CMD args…]
// JSONL directives on producer stdout (or contapila stdin if no --).
// With --, contapila stdin is passed through to CMD.
type ingestCmd struct {
	cmdFlags
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
	if err := c.apply(); err != nil {
		return err
	}
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
	cmdFlags
	Addr   cmd.StringArg `long:"addr" help:"listen address (host:port)" default:"127.0.0.1:8765"`
	Ledger *ledgerArg
}

func (webCmd) Description() string { return "Read-only web UI" }

func (c *webCmd) Run(ctx context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	cwd, err := projectCwd()
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
	cmdFlags
	Out  cmd.StringArg   `short:"o" long:"out" help:"output directory" default:"site"`
	Jobs cmd.IntArg[int] `short:"j" long:"jobs" help:"parallel render workers (0 = GOMAXPROCS)" default:"0"`
}

func (buildCmd) Description() string {
	return "Write a static HTML site from the project (no time filters)"
}

func (c *buildCmd) Run(ctx context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	cwd, err := projectCwd()
	if err != nil {
		return err
	}
	if err := web.Build(ctx, cwd, c.Out.Value(), c.Jobs.Value()); err != nil {
		return err
	}
	// Phase detail is on stderr via slog; keep a one-line stdout summary.
	fmt.Printf("wrote static site to %s\n", c.Out.Value())
	return nil
}

type lspCmd struct {
	cmdFlags
}

func (lspCmd) Description() string {
	return "Language server (stdio) for Helix and other LSP clients"
}

func (c *lspCmd) Run(ctx context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	// Protocol on stdout; keep slog on stderr.
	return lsp.RunStdio(ctx)
}
