package ingest

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/parser"
)

func mustRat(t *testing.T, s string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		t.Fatalf("invalid rat %q", s)
	}
	return r
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatal(err)
	}
	return tm.UTC()
}

func parseOne(t *testing.T, src string) ast.Directive {
	t.Helper()
	dirs, diags, err := parser.Parse("roundtrip.beancount", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v\nsrc:\n%s", err, src)
	}
	if diags.HasErrors() {
		t.Fatalf("diags: %s\nsrc:\n%s", diags.FormatErrors(), src)
	}
	if len(dirs) != 1 {
		t.Fatalf("want 1 directive, got %d\nsrc:\n%s", len(dirs), src)
	}
	return dirs[0]
}

func formatAndParse(t *testing.T, d ast.Directive) ast.Directive {
	t.Helper()
	src, err := FormatDirective(d)
	if err != nil {
		t.Fatal(err)
	}
	return parseOne(t, src)
}

func ratEq(t *testing.T, got *big.Rat, want string) {
	t.Helper()
	if got == nil {
		t.Fatalf("nil rat, want %s", want)
	}
	w := mustRat(t, want)
	if got.Cmp(w) != 0 {
		t.Fatalf("rat got %s want %s", got.FloatString(18), want)
	}
}

func TestFormatTransactionMultiPostingRoundTrip(t *testing.T) {
	txn := ast.Transaction{
		Meta:      ast.Meta{Date: mustDate(t, "2025-06-15")},
		Flag:      "*",
		Payee:     "Broker",
		Narration: "buy shares",
		Tags:      []string{"invest"},
		Links:     []string{"lot-1"},
		Metadata:  ast.Metadata{"source": "import", "batch": "june"},
		Postings: []ast.Posting{
			{
				Account: "Assets:Broker",
				Units:   &ast.Amount{Number: mustRat(t, "10"), Commodity: "HOOL"},
				Cost: &ast.CostSpec{
					Number:    mustRat(t, "100.5"),
					Commodity: "USD",
					Date:      mustDate(t, "2025-06-15"),
				},
				Metadata: ast.Metadata{"channel": "api"},
			},
			{
				Account: "Assets:Cash",
				Units:   &ast.Amount{Number: mustRat(t, "-1005"), Commodity: "USD"},
			},
			{
				// residual empty units leg
				Account: "Equity:Opening",
			},
		},
	}

	src, err := FormatDirective(txn)
	if err != nil {
		t.Fatal(err)
	}
	// sanity: cost date, residual line, metadata present
	if !strings.Contains(src, "Assets:Broker 10 HOOL {100.5 USD, 2025-06-15}") {
		t.Fatalf("unexpected broker line in:\n%s", src)
	}
	if !strings.Contains(src, "  Assets:Cash -1005 USD\n") {
		t.Fatalf("unexpected cash line in:\n%s", src)
	}
	if !strings.Contains(src, "  Equity:Opening\n") {
		t.Fatalf("missing residual posting in:\n%s", src)
	}
	if !strings.Contains(src, `  batch: "june"`) || !strings.Contains(src, `  source: "import"`) {
		t.Fatalf("missing txn metadata in:\n%s", src)
	}
	if !strings.Contains(src, `  channel: "api"`) {
		t.Fatalf("missing posting metadata in:\n%s", src)
	}

	got := parseOne(t, src).(ast.Transaction)
	if got.Payee != "Broker" || got.Narration != "buy shares" {
		t.Fatalf("header=%+v", got)
	}
	if got.Flag != "*" {
		t.Fatalf("flag=%q", got.Flag)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "invest" {
		t.Fatalf("tags=%v", got.Tags)
	}
	if len(got.Links) != 1 || got.Links[0] != "lot-1" {
		t.Fatalf("links=%v", got.Links)
	}
	if got.Metadata["source"] != "import" || got.Metadata["batch"] != "june" {
		t.Fatalf("meta=%v", got.Metadata)
	}
	if len(got.Postings) != 3 {
		t.Fatalf("postings=%d src:\n%s", len(got.Postings), src)
	}

	p0 := got.Postings[0]
	if p0.Account != "Assets:Broker" {
		t.Fatalf("p0 account=%s", p0.Account)
	}
	ratEq(t, p0.Units.Number, "10")
	if p0.Units.Commodity != "HOOL" {
		t.Fatalf("p0 units ccy=%s", p0.Units.Commodity)
	}
	if p0.Cost == nil || p0.Cost.Empty {
		t.Fatalf("p0 cost=%+v", p0.Cost)
	}
	ratEq(t, p0.Cost.Number, "100.5")
	if p0.Cost.Commodity != "USD" {
		t.Fatalf("p0 cost ccy=%s", p0.Cost.Commodity)
	}
	if p0.Cost.Date.Format("2006-01-02") != "2025-06-15" {
		t.Fatalf("p0 cost date=%v", p0.Cost.Date)
	}
	if p0.Metadata["channel"] != "api" {
		t.Fatalf("p0 meta=%v", p0.Metadata)
	}

	p1 := got.Postings[1]
	if p1.Account != "Assets:Cash" {
		t.Fatalf("p1 account=%s", p1.Account)
	}
	ratEq(t, p1.Units.Number, "-1005")
	if p1.Units.Commodity != "USD" {
		t.Fatalf("p1 ccy=%s", p1.Units.Commodity)
	}

	p2 := got.Postings[2]
	if p2.Account != "Equity:Opening" {
		t.Fatalf("p2 account=%s", p2.Account)
	}
	if p2.Units != nil {
		t.Fatalf("p2 residual units should be nil, got %+v", p2.Units)
	}
}

func TestFormatPostingEmptyCostAndTotalPrice(t *testing.T) {
	txn := ast.Transaction{
		Meta:      ast.Meta{Date: mustDate(t, "2025-03-01")},
		Flag:      "*",
		Narration: "edges",
		Postings: []ast.Posting{
			{
				Account: "Assets:Stock",
				Units:   &ast.Amount{Number: mustRat(t, "5"), Commodity: "HOOL"},
				Cost:    &ast.CostSpec{Empty: true},
				Price: &ast.PriceSpec{
					Number:    mustRat(t, "500"),
					Commodity: "USD",
					Total:     true, // @@
				},
			},
			{
				Account: "Assets:Cash",
				Units:   &ast.Amount{Number: mustRat(t, "-500"), Commodity: "USD"},
			},
		},
	}

	src, err := FormatDirective(txn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, " {}") {
		t.Fatalf("missing empty cost {} in:\n%s", src)
	}
	if !strings.Contains(src, " @@ 500 USD") {
		t.Fatalf("missing total price @@ in:\n%s", src)
	}

	got := parseOne(t, src).(ast.Transaction)
	p0 := got.Postings[0]
	if p0.Cost == nil || !p0.Cost.Empty {
		t.Fatalf("cost=%+v", p0.Cost)
	}
	if p0.Price == nil || !p0.Price.Total {
		t.Fatalf("price=%+v", p0.Price)
	}
	ratEq(t, p0.Price.Number, "500")
	if p0.Price.Commodity != "USD" {
		t.Fatalf("price ccy=%s", p0.Price.Commodity)
	}
}

func TestFormatPostingUnitPrice(t *testing.T) {
	txn := ast.Transaction{
		Meta:      ast.Meta{Date: mustDate(t, "2025-03-02")},
		Flag:      "*",
		Narration: "unit price",
		Postings: []ast.Posting{
			{
				Account: "Assets:Stock",
				Units:   &ast.Amount{Number: mustRat(t, "2"), Commodity: "HOOL"},
				Price: &ast.PriceSpec{
					Number:    mustRat(t, "50.25"),
					Commodity: "USD",
					Total:     false, // @
				},
			},
			{Account: "Assets:Cash"},
		},
	}
	src, err := FormatDirective(txn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, " @ 50.25 USD") {
		t.Fatalf("missing unit price @ in:\n%s", src)
	}
	got := parseOne(t, src).(ast.Transaction)
	p0 := got.Postings[0]
	if p0.Price == nil || p0.Price.Total {
		t.Fatalf("price=%+v", p0.Price)
	}
	ratEq(t, p0.Price.Number, "50.25")
}

func TestJSONLFormatParseRoundTripDirectives(t *testing.T) {
	jsonl := strings.Join([]string{
		`{"type":"open","date":"2025-01-01","id":"open:cash","account":"Assets:Cash","currencies":["BRL","USD"],"metadata":{"institution":"bank"}}`,
		`{"type":"balance","date":"2025-01-31","id":"bal:cash","account":"Assets:Cash","amount":{"number":"100.5","commodity":"BRL"},"metadata":{"check":"month-end"}}`,
		`{"type":"price","date":"2025-02-01","id":"px:usd","currency":"USD","amount":{"number":"5.25","commodity":"BRL"}}`,
		`{"type":"pad","date":"2025-02-02","id":"pad:eq","account":"Assets:Cash","from_account":"Equity:Opening-Balances"}`,
		`{"type":"transaction","date":"2025-02-03","id":"txn:1","flag":"!","payee":"Cafe","narration":"lunch","tags":["food"],"links":["receipt-9"],"metadata":{"note":"cash"},"postings":[{"account":"Expenses:Food","units":{"number":"12.5","commodity":"BRL"},"metadata":{"vat":"yes"}},{"account":"Assets:Cash","units":{"number":"-12.5","commodity":"BRL"}}]}`,
	}, "\n") + "\n"

	dirs, err := DecodeJSONL(strings.NewReader(jsonl), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 5 {
		t.Fatalf("n=%d", len(dirs))
	}

	// open
	openSrc, err := FormatDirective(dirs[0])
	if err != nil {
		t.Fatal(err)
	}
	open := parseOne(t, openSrc).(ast.Open)
	if open.Account != "Assets:Cash" {
		t.Fatalf("open account=%s", open.Account)
	}
	if len(open.Currencies) != 2 || open.Currencies[0] != "BRL" || open.Currencies[1] != "USD" {
		t.Fatalf("currencies=%v", open.Currencies)
	}
	if open.Metadata["institution"] != "bank" || open.Metadata[ast.IngestIDMetaKey] != "open:cash" {
		t.Fatalf("open meta=%v", open.Metadata)
	}

	// balance
	balSrc, err := FormatDirective(dirs[1])
	if err != nil {
		t.Fatal(err)
	}
	bal := parseOne(t, balSrc).(ast.Balance)
	if bal.Account != "Assets:Cash" {
		t.Fatalf("bal account=%s", bal.Account)
	}
	ratEq(t, bal.Amount.Number, "100.5")
	if bal.Amount.Commodity != "BRL" {
		t.Fatalf("bal ccy=%s", bal.Amount.Commodity)
	}
	if bal.Metadata["check"] != "month-end" || bal.Metadata[ast.IngestIDMetaKey] != "bal:cash" {
		t.Fatalf("bal meta=%v", bal.Metadata)
	}

	// price
	pxSrc, err := FormatDirective(dirs[2])
	if err != nil {
		t.Fatal(err)
	}
	px := parseOne(t, pxSrc).(ast.Price)
	if px.Currency != "USD" {
		t.Fatalf("px currency=%s", px.Currency)
	}
	ratEq(t, px.Amount.Number, "5.25")
	if px.Amount.Commodity != "BRL" {
		t.Fatalf("px quote=%s", px.Amount.Commodity)
	}
	if px.Metadata[ast.IngestIDMetaKey] != "px:usd" {
		t.Fatalf("px meta=%v", px.Metadata)
	}

	// pad
	padSrc, err := FormatDirective(dirs[3])
	if err != nil {
		t.Fatal(err)
	}
	pad := parseOne(t, padSrc).(ast.Pad)
	if pad.Account != "Assets:Cash" || pad.FromAccount != "Equity:Opening-Balances" {
		t.Fatalf("pad=%+v", pad)
	}
	if pad.Metadata[ast.IngestIDMetaKey] != "pad:eq" {
		t.Fatalf("pad meta=%v", pad.Metadata)
	}

	// transaction with postings via JSONL → Format → parse
	txnSrc, err := FormatDirective(dirs[4])
	if err != nil {
		t.Fatal(err)
	}
	txn := parseOne(t, txnSrc).(ast.Transaction)
	if txn.Flag != "!" || txn.Payee != "Cafe" || txn.Narration != "lunch" {
		t.Fatalf("txn header=%+v", txn)
	}
	if len(txn.Tags) != 1 || txn.Tags[0] != "food" {
		t.Fatalf("tags=%v", txn.Tags)
	}
	if len(txn.Links) != 1 || txn.Links[0] != "receipt-9" {
		t.Fatalf("links=%v", txn.Links)
	}
	if txn.Metadata["note"] != "cash" || txn.Metadata[ast.IngestIDMetaKey] != "txn:1" {
		t.Fatalf("txn meta=%v", txn.Metadata)
	}
	if len(txn.Postings) != 2 {
		t.Fatalf("postings=%d\n%s", len(txn.Postings), txnSrc)
	}
	ratEq(t, txn.Postings[0].Units.Number, "12.5")
	if txn.Postings[0].Account != "Expenses:Food" || txn.Postings[0].Units.Commodity != "BRL" {
		t.Fatalf("p0=%+v", txn.Postings[0])
	}
	if txn.Postings[0].Metadata["vat"] != "yes" {
		t.Fatalf("p0 meta=%v", txn.Postings[0].Metadata)
	}
	ratEq(t, txn.Postings[1].Units.Number, "-12.5")
	if txn.Postings[1].Account != "Assets:Cash" {
		t.Fatalf("p1=%+v", txn.Postings[1])
	}
}

func TestJSONLTransactionCostPriceRoundTrip(t *testing.T) {
	// Covers toPosting paths: units, empty cost, dated cost, unit price, total price, residual units.
	jsonl := strings.Join([]string{
		`{"type":"transaction","date":"2025-04-01","narration":"lot","postings":[{"account":"Assets:Broker","units":{"number":"3","commodity":"HOOL"},"cost":{"empty":true},"price":{"number":"10","commodity":"USD","total":false}},{"account":"Assets:Cash"}]}`,
		`{"type":"transaction","date":"2025-04-02","narration":"lot2","postings":[{"account":"Assets:Broker","units":{"number":"1","commodity":"HOOL"},"cost":{"number":"99.9","commodity":"USD","date":"2025-04-02"},"price":{"number":"100","commodity":"USD","total":true}},{"account":"Assets:Cash","units":{"number":"-100","commodity":"USD"}}]}`,
	}, "\n") + "\n"

	dirs, err := DecodeJSONL(strings.NewReader(jsonl), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 {
		t.Fatalf("n=%d", len(dirs))
	}

	// empty cost + unit price + residual
	t0 := dirs[0].(ast.Transaction)
	if t0.Postings[0].Cost == nil || !t0.Postings[0].Cost.Empty {
		t.Fatalf("decoded cost=%+v", t0.Postings[0].Cost)
	}
	if t0.Postings[0].Price == nil || t0.Postings[0].Price.Total {
		t.Fatalf("decoded price=%+v", t0.Postings[0].Price)
	}
	if t0.Postings[1].Units != nil {
		// JSON omitted units → residual after toPosting
		t.Fatalf("residual should have nil units, got %+v", t0.Postings[1].Units)
	}
	got0 := formatAndParse(t, t0).(ast.Transaction)
	if !got0.Postings[0].Cost.Empty {
		t.Fatalf("rt cost=%+v", got0.Postings[0].Cost)
	}
	if got0.Postings[0].Price.Total {
		t.Fatalf("rt price total=%v", got0.Postings[0].Price.Total)
	}
	ratEq(t, got0.Postings[0].Price.Number, "10")
	if got0.Postings[1].Units != nil {
		t.Fatalf("rt residual units=%+v", got0.Postings[1].Units)
	}

	// dated cost + total price
	t1 := dirs[1].(ast.Transaction)
	got1 := formatAndParse(t, t1).(ast.Transaction)
	p := got1.Postings[0]
	if p.Cost == nil || p.Cost.Empty {
		t.Fatalf("cost=%+v", p.Cost)
	}
	ratEq(t, p.Cost.Number, "99.9")
	if p.Cost.Commodity != "USD" || p.Cost.Date.Format("2006-01-02") != "2025-04-02" {
		t.Fatalf("cost=%+v date=%v", p.Cost, p.Cost.Date)
	}
	if p.Price == nil || !p.Price.Total {
		t.Fatalf("price=%+v", p.Price)
	}
	ratEq(t, p.Price.Number, "100")
}

func TestJSONLEmptyUnitsObjectIsResidual(t *testing.T) {
	// units present but both fields empty → toPosting leaves Units nil (residual).
	line := `{"type":"transaction","date":"2025-05-01","narration":"r","postings":[{"account":"Assets:Cash","units":{}},{"account":"Equity:Opening","units":{"number":"0","commodity":"BRL"}}]}` + "\n"
	dirs, err := DecodeJSONL(strings.NewReader(line), nil)
	if err != nil {
		t.Fatal(err)
	}
	txn := dirs[0].(ast.Transaction)
	if txn.Postings[0].Units != nil {
		t.Fatalf("want residual nil units, got %+v", txn.Postings[0].Units)
	}
	src, err := FormatDirective(txn)
	if err != nil {
		t.Fatal(err)
	}
	// residual formats as bare account line
	if !strings.Contains(src, "  Assets:Cash\n") {
		t.Fatalf("src:\n%s", src)
	}
	got := parseOne(t, src).(ast.Transaction)
	if got.Postings[0].Units != nil {
		t.Fatalf("parsed residual units=%+v", got.Postings[0].Units)
	}
}
