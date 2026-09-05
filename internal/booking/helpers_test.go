package booking

import (
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/ast"
)

func TestValidateAutoInterestRates(t *testing.T) {
	t.Run("good rate empty diags", func(t *testing.T) {
		diags := ValidateAutoInterestRates([]ast.Directive{
			ast.Open{
				Meta:     ast.Meta{Date: d("2025-01-01"), File: "t", Line: 1},
				Account:  "Assets:CDB",
				Metadata: ast.Metadata{"interest_rate": "115% CDI"},
			},
		})
		if len(diags) != 0 {
			t.Fatalf("diags=%v want empty", diags)
		}
	})

	t.Run("garbage rate error with account", func(t *testing.T) {
		diags := ValidateAutoInterestRates([]ast.Directive{
			ast.Open{
				Meta:     ast.Meta{Date: d("2025-01-01"), File: "t", Line: 2},
				Account:  "Assets:CDB:Bad",
				Metadata: ast.Metadata{"interest_rate": "garbage"},
			},
		})
		if !diags.HasErrors() {
			t.Fatal("expected error diag")
		}
		msg := diags.Format()
		if !strings.Contains(msg, "Assets:CDB:Bad") {
			t.Fatalf("diag missing account: %s", msg)
		}
		if !strings.Contains(msg, "garbage") {
			t.Fatalf("diag missing rate: %s", msg)
		}
	})

	t.Run("interest-rate alias", func(t *testing.T) {
		diags := ValidateAutoInterestRates([]ast.Directive{
			ast.Open{
				Meta:     ast.Meta{Date: d("2025-01-01"), File: "t", Line: 3},
				Account:  "Assets:CDB:Alias",
				Metadata: ast.Metadata{"interest-rate": "100% CDI"},
			},
		})
		if len(diags) != 0 {
			t.Fatalf("good alias diags=%v want empty", diags)
		}

		diags = ValidateAutoInterestRates([]ast.Directive{
			ast.Open{
				Meta:     ast.Meta{Date: d("2025-01-01"), File: "t", Line: 4},
				Account:  "Assets:CDB:AliasBad",
				Metadata: ast.Metadata{"interest-rate": "not-a-rate"},
			},
		})
		if !diags.HasErrors() {
			t.Fatal("expected error on bad alias rate")
		}
		if !strings.Contains(diags.Format(), "Assets:CDB:AliasBad") {
			t.Fatalf("diag missing account: %s", diags.Format())
		}
	})

	t.Run("empty meta skip", func(t *testing.T) {
		diags := ValidateAutoInterestRates([]ast.Directive{
			ast.Open{Meta: ast.Meta{Date: d("2025-01-01"), File: "t", Line: 5}, Account: "Assets:Cash"},
			ast.Open{
				Meta:     ast.Meta{Date: d("2025-01-01"), File: "t", Line: 6},
				Account:  "Assets:CDB:Empty",
				Metadata: ast.Metadata{"interest_rate": ""},
			},
			ast.Open{
				Meta:     ast.Meta{Date: d("2025-01-01"), File: "t", Line: 7},
				Account:  "Assets:CDB:Spaces",
				Metadata: ast.Metadata{"interest_rate": "   ", "interest-rate": "  "},
			},
			ast.Transaction{
				Meta: ast.Meta{Date: d("2025-01-01"), File: "t", Line: 8},
				Flag: "*", Narration: "not an open",
			},
		})
		if len(diags) != 0 {
			t.Fatalf("diags=%v want empty", diags)
		}
	})
}

func TestAllBalances(t *testing.T) {
	e := New()
	e.Book([]ast.Directive{
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Assets:Cash"},
		ast.Open{Meta: ast.Meta{Date: d("2020-01-01")}, Account: "Equity:Opening"},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-02"), File: "t"},
			Flag: "*", Narration: "seed",
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: amt("100", "BRL")},
				{Account: "Equity:Opening", Units: amt("-100", "BRL")},
			},
		},
		// Round-trip USD so Bal keeps a zero entry that AllBalances must omit.
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-03"), File: "t"},
			Flag: "*", Narration: "usd in",
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: amt("5", "USD")},
				{Account: "Equity:Opening", Units: amt("-5", "USD")},
			},
		},
		ast.Transaction{
			Meta: ast.Meta{Date: d("2020-01-04"), File: "t"},
			Flag: "*", Narration: "usd out",
			Postings: []ast.Posting{
				{Account: "Assets:Cash", Units: amt("-5", "USD")},
				{Account: "Equity:Opening", Units: amt("5", "USD")},
			},
		},
	})
	if e.Diags.HasErrors() {
		t.Fatalf("book: %v", e.Diags)
	}

	// Sanity: engine still holds zero USD slots.
	if got := e.balOf("Assets:Cash", "USD"); got.Sign() != 0 {
		t.Fatalf("internal USD bal=%s want 0", got.FloatString(4))
	}

	all := e.AllBalances()
	cash := all["Assets:Cash"]
	if cash == nil {
		t.Fatal("missing Assets:Cash")
	}
	if cash["BRL"] == nil || cash["BRL"].Cmp(r("100")) != 0 {
		t.Fatalf("Assets:Cash BRL=%v want 100", cash["BRL"])
	}
	if _, ok := cash["USD"]; ok {
		t.Fatal("zero USD must be omitted from AllBalances")
	}

	eq := all["Equity:Opening"]
	if eq == nil {
		t.Fatal("missing Equity:Opening")
	}
	if eq["BRL"] == nil || eq["BRL"].Cmp(r("-100")) != 0 {
		t.Fatalf("Equity:Opening BRL=%v want -100", eq["BRL"])
	}
	if _, ok := eq["USD"]; ok {
		t.Fatal("zero Equity USD must be omitted from AllBalances")
	}
}

func TestAccountClassHelpers(t *testing.T) {
	cases := []struct {
		account   string
		income    bool
		expense   bool
		asset     bool
		liability bool
	}{
		{"Income:Salary", true, false, false, false},
		{"Expenses:Food", false, true, false, false},
		{"Assets:Cash", false, false, true, false},
		{"Liabilities:Card", false, false, false, true},
		{"Equity:Opening", false, false, false, false},
		{"Income", false, false, false, false},
		{"Expenses", false, false, false, false},
		{"Assets", false, false, false, false},
		{"Liabilities", false, false, false, false},
		{"Expenses:Food:Lunch", false, true, false, false},
		{"Assets:BR:Bank", false, false, true, false},
		{"Income:Passivo:CDB", true, false, false, false},
	}
	for _, tc := range cases {
		if got := IsIncome(tc.account); got != tc.income {
			t.Errorf("IsIncome(%q)=%v want %v", tc.account, got, tc.income)
		}
		if got := IsExpense(tc.account); got != tc.expense {
			t.Errorf("IsExpense(%q)=%v want %v", tc.account, got, tc.expense)
		}
		if got := IsAsset(tc.account); got != tc.asset {
			t.Errorf("IsAsset(%q)=%v want %v", tc.account, got, tc.asset)
		}
		if got := IsLiability(tc.account); got != tc.liability {
			t.Errorf("IsLiability(%q)=%v want %v", tc.account, got, tc.liability)
		}
	}
}
