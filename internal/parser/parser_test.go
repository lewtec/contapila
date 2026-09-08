package parser

import (
	"math/big"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/ast"
)

func rRat(s string) *big.Rat {
	x, ok := new(big.Rat).SetString(s)
	if !ok {
		panic(s)
	}
	return x
}

func TestParseBasic(t *testing.T) {
	src := []byte(`
2020-01-01 open Assets:Cash
2020-01-01 open Expenses:Food
2020-01-02 * "Lunch"
  Assets:Cash  -30.00 BRL
  Expenses:Food
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags: %v", diags)
	}
	var opens, txns int
	for _, d := range dirs {
		switch d.(type) {
		case ast.Open:
			opens++
		case ast.Transaction:
			txns++
		}
	}
	if opens != 2 || txns != 1 {
		t.Fatalf("opens=%d txns=%d dirs=%d", opens, txns, len(dirs))
	}
}

func TestParseOpenCurrencyAndMetaWarn(t *testing.T) {
	src := []byte(`
2020-01-01 open Assets:Cash BRL
  institution: "Banco"
2020-01-02 * "Lunch" #todo
  role: "meal"
  Assets:Cash  -30.00BRL
  Expenses:Food
2020-01-03 query "q" "select *"
2020-01-04 custom "fava-option" "True"
2020-01-05 document Assets:Cash "personal/docs/by-account/Assets/Cash/20200105_x.txt"
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	var open ast.Open
	var txn ast.Transaction
	var doc ast.Document
	for _, d := range dirs {
		switch v := d.(type) {
		case ast.Open:
			open = v
		case ast.Transaction:
			txn = v
		case ast.Document:
			doc = v
		}
	}
	if open.Account != "Assets:Cash" || len(open.Currencies) != 1 || open.Currencies[0] != "BRL" {
		t.Fatalf("open=%+v", open)
	}
	if open.Metadata["institution"] != "Banco" {
		t.Fatalf("open metadata=%v", open.Metadata)
	}
	if len(txn.Tags) != 1 || txn.Tags[0] != "todo" {
		t.Fatalf("tags=%v", txn.Tags)
	}
	if txn.Postings[0].Units == nil || txn.Postings[0].Units.Commodity != "BRL" {
		t.Fatalf("posting units=%+v", txn.Postings[0].Units)
	}
	if doc.Account != "Assets:Cash" || doc.Path != "personal/docs/by-account/Assets/Cash/20200105_x.txt" {
		t.Fatalf("document=%+v", doc)
	}
	// open + txn metadata stored; query still warn; custom parses; document parses
	if txn.Metadata["role"] != "meal" {
		t.Fatalf("txn metadata=%v", txn.Metadata)
	}
	var hasQuery bool
	var custom ast.Custom
	var hasCustomDir bool
	for _, d := range dirs {
		if v, ok := d.(ast.Custom); ok {
			custom = v
			hasCustomDir = true
		}
	}
	for _, d := range diags {
		if strings.Contains(d.Message, "metadata") {
			t.Fatalf("metadata should be stored, not warned: %v", d.Message)
		}
		if strings.Contains(d.Message, "query") {
			hasQuery = true
		}
		if strings.Contains(d.Message, "document") {
			t.Fatalf("document should not warn-skip: %v", d.Message)
		}
	}
	if !hasQuery {
		t.Fatalf("expected query warn, diags=%v", diags)
	}
	if !hasCustomDir || custom.Type != "fava-option" {
		t.Fatalf("expected custom directive, got has=%v custom=%+v", hasCustomDir, custom)
	}
}

func TestParseCustomIndex(t *testing.T) {
	src := []byte(`2025-04-03 custom "index" "CDI" 0.000451
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags=%v", diags)
	}
	var c ast.Custom
	for _, d := range dirs {
		if v, ok := d.(ast.Custom); ok {
			c = v
		}
	}
	if c.Type != "index" || len(c.Values) < 2 {
		t.Fatalf("custom=%+v", c)
	}
	if c.Values[0].Text != "CDI" {
		t.Fatalf("indicator=%q", c.Values[0].Text)
	}
	if c.Values[1].Number == nil || c.Values[1].Number.Cmp(rRat("0.000451")) != 0 {
		t.Fatalf("rate=%v", c.Values[1].Number)
	}
}

func TestParseTxnAndPostingMetadata(t *testing.T) {
	src := []byte(`
2020-01-01 open Assets:Cash
2020-01-01 open Expenses:Food
2020-01-05 * "Shop" "Groceries"
  invoice: "INV-1"
  document: "personal/docs/by-account/Expenses/Food/20200105_x.txt"
  Assets:Cash  -30.00 BRL
    channel: "card"
  Expenses:Food
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range diags {
		if strings.Contains(d.Message, "metadata") {
			t.Fatalf("unexpected meta warn: %v", d.Message)
		}
	}
	var txn ast.Transaction
	for _, d := range dirs {
		if v, ok := d.(ast.Transaction); ok {
			txn = v
		}
	}
	if txn.Metadata["invoice"] != "INV-1" || txn.Metadata["document"] == "" {
		t.Fatalf("txn meta=%v", txn.Metadata)
	}
	if len(txn.Postings) < 1 || txn.Postings[0].Metadata["channel"] != "card" {
		t.Fatalf("postings=%+v", txn.Postings)
	}
}

func TestParseCommodityMetadata(t *testing.T) {
	src := []byte(`
1860-01-01 commodity BRL
  name: "Brazilian Real"
  asset-class: "fiat"
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags: %v", diags)
	}
	for _, d := range diags {
		if strings.Contains(d.Message, "metadata") {
			t.Fatalf("commodity meta should be stored: %v", d.Message)
		}
	}
	var c ast.Commodity
	for _, d := range dirs {
		if v, ok := d.(ast.Commodity); ok {
			c = v
		}
	}
	if c.Currency != "BRL" || c.Metadata["name"] != "Brazilian Real" || c.Metadata["asset-class"] != "fiat" {
		t.Fatalf("commodity=%+v meta=%v", c, c.Metadata)
	}
}

func TestParsePriceMetadata(t *testing.T) {
	src := []byte(`
2024-01-15 price B3_PETR4 38.50 BRL
  source: "B3"
  note: "close"
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags: %v", diags)
	}
	for _, d := range diags {
		if strings.Contains(d.Message, "metadata") {
			t.Fatalf("price meta should be stored: %v", d.Message)
		}
	}
	var p ast.Price
	for _, d := range dirs {
		if v, ok := d.(ast.Price); ok {
			p = v
		}
	}
	if p.Currency != "B3_PETR4" || p.Amount.Commodity != "BRL" {
		t.Fatalf("price=%+v", p)
	}
	if p.Metadata["source"] != "B3" || p.Metadata["note"] != "close" {
		t.Fatalf("price meta=%v", p.Metadata)
	}
}

func TestParseEventMetadata(t *testing.T) {
	src := []byte(`
2020-01-01 event "location" "SF"
  city: "San Francisco"
  source: "manual"
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags: %v", diags)
	}
	for _, d := range diags {
		if strings.Contains(d.Message, "metadata") || strings.Contains(d.Message, "key") {
			t.Fatalf("event meta should be stored: %v", d.Message)
		}
	}
	var ev ast.Event
	found := false
	for _, d := range dirs {
		if v, ok := d.(ast.Event); ok {
			ev = v
			found = true
		}
	}
	if !found {
		t.Fatal("no event")
	}
	if ev.Type != "location" || ev.Desc != "SF" {
		t.Fatalf("event=%+v", ev)
	}
	if ev.Metadata["city"] != "San Francisco" || ev.Metadata["source"] != "manual" {
		t.Fatalf("event meta=%v", ev.Metadata)
	}
}

func TestParseBalanceMetadata(t *testing.T) {
	src := []byte(`
2020-01-01 balance Assets:Cash 100.00 BRL
  statement: "bank"
  note: "eom"
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags: %v", diags)
	}
	for _, d := range diags {
		if strings.Contains(d.Message, "metadata") || strings.Contains(d.Message, "key") {
			t.Fatalf("balance meta should be stored: %v", d.Message)
		}
	}
	var b ast.Balance
	found := false
	for _, d := range dirs {
		if v, ok := d.(ast.Balance); ok {
			b = v
			found = true
		}
	}
	if !found {
		t.Fatal("no balance")
	}
	if b.Account != "Assets:Cash" || b.Amount.Commodity != "BRL" {
		t.Fatalf("balance=%+v", b)
	}
	if b.Metadata["statement"] != "bank" || b.Metadata["note"] != "eom" {
		t.Fatalf("balance meta=%v", b.Metadata)
	}
}

func TestParseSectionCollectsNested(t *testing.T) {
	src := []byte(`
* Assets section
2020-01-01 open Assets:Cash BRL
** Nested
2020-01-02 open Expenses:Food
; real comment
2020-01-03 balance Assets:Cash 0 BRL
  checked: "TRUE"
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags: %v", diags)
	}
	for _, d := range diags {
		if strings.Contains(d.Message, "section") {
			t.Fatalf("section should be silent structure: %v", d.Message)
		}
	}
	var opens, bals int
	var bal ast.Balance
	for _, d := range dirs {
		switch v := d.(type) {
		case ast.Open:
			opens++
		case ast.Balance:
			bals++
			bal = v
		}
	}
	if opens != 2 || bals != 1 {
		t.Fatalf("opens=%d bals=%d dirs=%d", opens, bals, len(dirs))
	}
	if bal.Metadata["checked"] != "TRUE" {
		t.Fatalf("balance meta under section=%v", bal.Metadata)
	}
}

func TestParsePayeeAndNarration(t *testing.T) {
	src := []byte(`
2020-01-01 open Assets:Cash
2020-01-01 open Expenses:Food
2020-01-05 * "Restaurant Foo" "Lunch"
  Assets:Cash  -30.00 BRL
  Expenses:Food
2020-01-06 * "Coffee only narration"
  Assets:Cash  -10.00 BRL
  Expenses:Food
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags: %v", diags)
	}
	var both, narrOnly *ast.Transaction
	for _, d := range dirs {
		txn, ok := d.(ast.Transaction)
		if !ok {
			continue
		}
		switch txn.Date.Format("2006-01-02") {
		case "2020-01-05":
			t := txn
			both = &t
		case "2020-01-06":
			t := txn
			narrOnly = &t
		}
	}
	if both == nil || narrOnly == nil {
		t.Fatalf("missing txns both=%v narrOnly=%v", both, narrOnly)
	}
	if both.Payee != "Restaurant Foo" || both.Narration != "Lunch" {
		t.Fatalf("payee+narration: payee=%q narration=%q", both.Payee, both.Narration)
	}
	if narrOnly.Payee != "" || narrOnly.Narration != "Coffee only narration" {
		t.Fatalf("narration-only: payee=%q narration=%q", narrOnly.Payee, narrOnly.Narration)
	}
}

// Unparseable amount expressions (e.g. 1/0) still form price/balance nodes in the CST;
// convert must emit an error and skip rather than stream a zero amount.
func TestParsePriceInvalidAmountSkipped(t *testing.T) {
	src := []byte(`
2024-01-15 price B3_PETR4 1/0 BRL
2024-01-16 price USD 5.00 BRL
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if !diags.HasErrors() {
		t.Fatal("expected error for invalid price amount")
	}
	var found bool
	for _, d := range diags {
		if d.IsError() && strings.Contains(d.Message, "price") && strings.Contains(d.Message, "amount") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected price amount error, got %v", diags)
	}
	var prices []ast.Price
	for _, d := range dirs {
		if p, ok := d.(ast.Price); ok {
			prices = append(prices, p)
		}
	}
	if len(prices) != 1 || prices[0].Currency != "USD" {
		t.Fatalf("want only valid USD price, got %+v", prices)
	}
}

func TestParseBalanceInvalidAmountSkipped(t *testing.T) {
	src := []byte(`
2020-01-01 balance Assets:Cash 1/0 BRL
2020-01-02 balance Assets:Cash 100.00 BRL
`)
	dirs, diags, err := Parse("t.beancount", src)
	if err != nil {
		t.Fatal(err)
	}
	if !diags.HasErrors() {
		t.Fatal("expected error for invalid balance amount")
	}
	var found bool
	for _, d := range diags {
		if d.IsError() && strings.Contains(d.Message, "balance") && strings.Contains(d.Message, "amount") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected balance amount error, got %v", diags)
	}
	var bals []ast.Balance
	for _, d := range dirs {
		if b, ok := d.(ast.Balance); ok {
			bals = append(bals, b)
		}
	}
	if len(bals) != 1 || bals[0].Amount.Number == nil || bals[0].Amount.Number.Cmp(rRat("100")) != 0 {
		t.Fatalf("want only valid 100 BRL balance, got %+v", bals)
	}
}
