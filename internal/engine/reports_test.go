package engine

import (
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openExamplePersonal(t *testing.T) *Ledger {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "example")
	p, pdb, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(p, pdb, "personal")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestLedgerNames(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "example")
	p, _, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	names := LedgerNames(p)
	if len(names) == 0 {
		t.Fatal("expected ledger names")
	}
	want := map[string]bool{"personal": true, "acme": true, "ong": true, "smuggle": true}
	for _, n := range names {
		if !want[n] {
			t.Fatalf("unexpected ledger %q in %v", n, names)
		}
	}
	for n := range want {
		found := false
		for _, got := range names {
			if got == n {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing ledger %q in %v", n, names)
		}
	}
	// sorted
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Fatalf("names not sorted: %v", names)
		}
	}
}

func TestParseDate(t *testing.T) {
	// Empty → zero time (unbounded/open range convention used by journal/PnL filters).
	got, err := ParseDate("")
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("empty want zero time, got %v", got)
	}

	got, err = ParseDate("2024-03-15")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}

	if _, err := ParseDate("not-a-date"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestJournal(t *testing.T) {
	l := openExamplePersonal(t)

	all := l.Journal(time.Time{}, time.Time{})
	if len(all) == 0 {
		t.Fatal("expected journal entries in personal")
	}
	// Dates monotonic non-decreasing
	for i := 1; i < len(all); i++ {
		if all[i].Date.Before(all[i-1].Date) {
			t.Fatalf("journal not ordered: %s before %s", all[i].Date, all[i-1].Date)
		}
	}
	// At least some booked txns with postings
	var txnCount int
	for _, e := range all {
		if e.Kind == "txn" {
			txnCount++
			if len(e.Postings) < 2 {
				t.Fatalf("txn %q on %s has %d postings", e.Narration, e.Date.Format("2006-01-02"), len(e.Postings))
			}
		}
	}
	if txnCount == 0 {
		t.Fatal("expected booked txns")
	}

	// In-range filter (2024)
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	y2024 := l.Journal(from, to)
	if len(y2024) == 0 {
		t.Fatal("expected 2024 journal activity")
	}
	for _, e := range y2024 {
		if e.Date.Before(from) || e.Date.After(to) {
			t.Fatalf("entry %s outside [2024]", e.Date.Format("2006-01-02"))
		}
	}
	if len(y2024) >= len(all) {
		t.Fatalf("2024 filter should be stricter than unbounded: 2024=%d all=%d", len(y2024), len(all))
	}

	// Out-of-range: far future → empty
	far := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	empty := l.Journal(far, time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC))
	if len(empty) != 0 {
		t.Fatalf("far-future journal want 0, got %d", len(empty))
	}

	// Before ledger starts (example opens mid-2023)
	early := l.Journal(
		time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2010, 12, 31, 0, 0, 0, 0, time.UTC),
	)
	if len(early) != 0 {
		t.Fatalf("pre-ledger journal want 0, got %d", len(early))
	}
}

func TestJournalForAccount(t *testing.T) {
	l := openExamplePersonal(t)
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	acct := "Assets:BR:Alfa:ContaCorrente"
	entries := l.JournalForAccount(acct, from, to)
	if len(entries) == 0 {
		t.Fatalf("expected journal activity on %s in 2024", acct)
	}
	for _, e := range entries {
		if e.Kind == "txn" {
			touch := false
			for _, p := range e.Postings {
				if AccountMatches(p.Account, acct) {
					touch = true
					break
				}
			}
			if !touch {
				t.Fatalf("txn %q does not touch %s", e.Narration, acct)
			}
		} else if e.Kind == "note" {
			if !AccountMatches(e.Account, acct) {
				t.Fatalf("note on %s does not match filter %s", e.Account, acct)
			}
		}
	}

	// Prefix match: parent Assets:BR:Alfa should include ContaCorrente activity
	parent := l.JournalForAccount("Assets:BR:Alfa", from, to)
	if len(parent) < len(entries) {
		t.Fatalf("parent filter should include child entries: parent=%d child=%d", len(parent), len(entries))
	}

	// Unknown account → empty
	none := l.JournalForAccount("Assets:NoSuch:Account", from, to)
	if len(none) != 0 {
		t.Fatalf("unknown account want 0, got %d", len(none))
	}
}

func TestJournalForCommodity(t *testing.T) {
	l := openExamplePersonal(t)
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	brl := l.JournalForCommodity("BRL", from, to)
	if len(brl) == 0 {
		t.Fatal("expected BRL journal activity")
	}
	for _, e := range brl {
		if e.Kind != "txn" {
			t.Fatalf("commodity journal should be txns only, got %s", e.Kind)
		}
		touch := false
		for _, p := range e.Postings {
			if p.Units != nil && p.Units.Commodity == "BRL" {
				touch = true
				break
			}
		}
		if !touch {
			t.Fatalf("entry %q has no BRL posting", e.Narration)
		}
	}

	// Dates ordered
	for i := 1; i < len(brl); i++ {
		if brl[i].Date.Before(brl[i-1].Date) {
			t.Fatal("commodity journal not ordered")
		}
	}

	// Rare/absent commodity
	none := l.JournalForCommodity("ZZZ_NOPE", from, to)
	if len(none) != 0 {
		t.Fatalf("absent commodity want 0, got %d", len(none))
	}
}

func TestAccountBalancesAndActivity(t *testing.T) {
	l := openExamplePersonal(t)
	acct := "Assets:BR:Alfa:ContaCorrente"

	bals := l.AccountBalances(acct, AsOfLatest)
	if len(bals) == 0 {
		t.Fatalf("expected balances for %s", acct)
	}
	brl, ok := bals["BRL"]
	if !ok || brl == nil {
		t.Fatal("expected BRL balance")
	}
	// Checking account with salary/expenses still positive in dogfood
	if brl.Sign() <= 0 {
		t.Fatalf("expected positive BRL balance, got %s", brl.FloatString(2))
	}

	// Subaccount-only filter: parent without exact match returns empty map (exact account only)
	parentOnly := l.AccountBalances("Assets:BR:Alfa", AsOfLatest)
	if len(parentOnly) != 0 {
		t.Fatalf("AccountBalances is exact-match only; parent got %v", parentOnly)
	}

	// Activity over a known salary window
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
	act := l.AccountActivity(acct, from, to)
	if len(act) == 0 {
		t.Fatal("expected January activity on ContaCorrente")
	}
	// January has salary +8500 BRL and various expenses — net may be anything; just require BRL key
	if act["BRL"] == nil {
		t.Fatal("expected BRL activity")
	}

	// Out of range activity empty
	far := l.AccountActivity(acct, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC))
	if len(far) != 0 {
		// zero-only maps may still appear only if postings exist; none should
		for _, n := range far {
			if n.Sign() != 0 {
				t.Fatalf("far-future activity non-zero: %s", n.FloatString(2))
			}
		}
	}
}

func TestCommodityBalancesAndActivity(t *testing.T) {
	l := openExamplePersonal(t)

	byAcct := l.CommodityBalances("BRL", AsOfLatest)
	if len(byAcct) == 0 {
		t.Fatal("expected BRL balances across accounts")
	}
	// Known cash accounts from opening
	if _, ok := byAcct["Assets:BR:Alfa:ContaCorrente"]; !ok {
		t.Fatal("missing ContaCorrente in BRL commodity balances")
	}
	for acct, n := range byAcct {
		if n == nil || n.Sign() == 0 {
			t.Fatalf("zero balance listed for %s", acct)
		}
		if !strings.HasPrefix(acct, "Assets:") && !strings.HasPrefix(acct, "Liabilities:") &&
			!strings.HasPrefix(acct, "Equity:") && !strings.HasPrefix(acct, "Income:") &&
			!strings.HasPrefix(acct, "Expenses:") {
			t.Fatalf("unexpected account name %q", acct)
		}
	}

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	act := l.CommodityActivity("BRL", from, to)
	if len(act) == 0 {
		t.Fatal("expected BRL activity in 2024")
	}
	// Income and expense accounts should appear with non-zero flows
	var sawIncome, sawExpense bool
	for acct := range act {
		if strings.HasPrefix(acct, "Income:") {
			sawIncome = true
		}
		if strings.HasPrefix(acct, "Expenses:") {
			sawExpense = true
		}
	}
	if !sawIncome || !sawExpense {
		t.Fatalf("commodity activity should touch Income and Expenses (income=%v expense=%v)", sawIncome, sawExpense)
	}

	none := l.CommodityBalances("ZZZ_NOPE", AsOfLatest)
	if len(none) != 0 {
		t.Fatalf("absent commodity balances want 0, got %d", len(none))
	}
}

func TestPnL(t *testing.T) {
	l := openExamplePersonal(t)
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	pnl := l.PnL(from, to)
	if len(pnl.Income) == 0 {
		t.Fatal("expected income accounts in 2024 PnL")
	}
	if len(pnl.Expenses) == 0 {
		t.Fatal("expected expense accounts in 2024 PnL")
	}

	// Salary account present; income amounts are negative in Beancount convention
	sal, ok := pnl.Income["Income:Ativo:BR:Salary"]
	if !ok {
		t.Fatal("missing Income:Ativo:BR:Salary")
	}
	if sal["BRL"] == nil || sal["BRL"].Sign() >= 0 {
		t.Fatalf("salary BRL should be negative (credit), got %v", sal["BRL"])
	}

	// Rent expense positive
	rent, ok := pnl.Expenses["Expenses:CustoFixo:Aluguel"]
	if !ok {
		t.Fatal("missing Expenses:CustoFixo:Aluguel")
	}
	if rent["BRL"] == nil || rent["BRL"].Sign() <= 0 {
		t.Fatalf("rent BRL should be positive (debit), got %v", rent["BRL"])
	}

	// Out-of-range empty
	empty := l.PnL(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC))
	if len(empty.Income) != 0 || len(empty.Expenses) != 0 {
		t.Fatalf("far-future PnL should be empty: income=%d expenses=%d", len(empty.Income), len(empty.Expenses))
	}

	// Unbounded includes more years than 2024 alone
	all := l.PnL(time.Time{}, time.Time{})
	if len(all.Income) < len(pnl.Income) {
		// income account set may grow with more years
		t.Logf("unbounded income accounts=%d 2024=%d", len(all.Income), len(pnl.Income))
	}
	// At least salary total magnitude should be larger unbounded (multi-year salaries)
	allSal := all.Income["Income:Ativo:BR:Salary"]["BRL"]
	ySal := sal["BRL"]
	if allSal == nil || ySal == nil {
		t.Fatal("salary missing in unbounded or 2024")
	}
	// More years → more negative (or equal if only 2024 has salary — dogfood has 2023+)
	if allSal.Cmp(ySal) > 0 {
		t.Fatalf("unbounded salary %s should be <= 2024 salary %s (more negative or equal)", allSal.FloatString(2), ySal.FloatString(2))
	}
}

func TestPnLTree(t *testing.T) {
	l := openExamplePersonal(t)
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	income, expenses := l.PnLTree(from, to)
	if len(income) == 0 {
		t.Fatal("expected hierarchical income lines")
	}
	if len(expenses) == 0 {
		t.Fatal("expected hierarchical expense lines")
	}

	// Roots present as rollups
	var sawIncomeRoot, sawExpenseRoot bool
	for _, ln := range income {
		if ln.Account == "Income" {
			sawIncomeRoot = true
			if !ln.IsRollup {
				t.Fatal("Income root should be rollup")
			}
			if ln.Amount == nil || ln.Amount.Sign() == 0 {
				t.Fatal("Income root amount should be non-zero")
			}
			// converted to op currency
			if l.OpCurrency != "" && ln.Commodity != l.OpCurrency {
				t.Fatalf("income commodity=%q want op %q", ln.Commodity, l.OpCurrency)
			}
		}
		if strings.Contains(ln.Name, ":") {
			t.Fatalf("Name should be leaf segment, got %q", ln.Name)
		}
		if ln.Depth < 0 {
			t.Fatalf("bad depth %d on %s", ln.Depth, ln.Account)
		}
	}
	for _, ln := range expenses {
		if ln.Account == "Expenses" {
			sawExpenseRoot = true
			if !ln.IsRollup {
				t.Fatal("Expenses root should be rollup")
			}
			if ln.Amount == nil || ln.Amount.Sign() == 0 {
				t.Fatal("Expenses root amount should be non-zero")
			}
		}
	}
	if !sawIncomeRoot {
		t.Fatal("missing Income root in PnLTree")
	}
	if !sawExpenseRoot {
		t.Fatal("missing Expenses root in PnLTree")
	}

	// Empty range
	ei, ee := l.PnLTree(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC))
	if len(ei) != 0 || len(ee) != 0 {
		t.Fatalf("far-future tree want empty, got income=%d expenses=%d", len(ei), len(ee))
	}
}

func TestAccountMatches(t *testing.T) {
	cases := []struct {
		acct, filter string
		want         bool
	}{
		{"Assets:Cash", "", true},
		{"Assets:Cash", "Assets:Cash", true},
		{"Assets:Cash:Wallet", "Assets:Cash", true},
		{"Assets:Cashier", "Assets:Cash", false},
		{"Assets:Cash", "Assets:Bank", false},
		{"Income:Ativo:BR:Salary", "Income:Ativo", true},
	}
	for _, tc := range cases {
		if got := AccountMatches(tc.acct, tc.filter); got != tc.want {
			t.Fatalf("AccountMatches(%q,%q)=%v want %v", tc.acct, tc.filter, got, tc.want)
		}
	}
}

// Ensure maps use independent big.Rat values (mutation safety light check).
func TestAccountBalancesCopy(t *testing.T) {
	l := openExamplePersonal(t)
	acct := "Assets:BR:Alfa:ContaCorrente"
	b1 := l.AccountBalances(acct, AsOfLatest)
	if b1["BRL"] == nil {
		t.Fatal("missing BRL")
	}
	orig := new(big.Rat).Set(b1["BRL"])
	b1["BRL"].Add(b1["BRL"], big.NewRat(1, 1))
	b2 := l.AccountBalances(acct, AsOfLatest)
	if b2["BRL"].Cmp(orig) != 0 {
		t.Fatalf("mutating returned map affected ledger state: got %s want %s", b2["BRL"].FloatString(2), orig.FloatString(2))
	}
}
