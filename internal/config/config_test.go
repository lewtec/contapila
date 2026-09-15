package config

import (
	"os"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

func TestLoad(t *testing.T) {
	t.Run("empty config", func(t *testing.T) {
		cfg, err := Load([]byte("{}"), "test.cue", nil, nil)
		if err != nil {
			t.Fatalf("failed to load empty config: %v", err)
		}
		val := cfg.Value.LookupPath(cue.ParsePath("commodities"))
		if !val.Exists() {
			t.Errorf("expected commodities to exist in unified config")
		}
		// empty discovery → closed empty ledgers
		ledgers := cfg.Value.LookupPath(cue.ParsePath("ledgers"))
		if !ledgers.Exists() {
			t.Fatal("expected ledgers map")
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		_, err := Load([]byte("invalid cue"), "test.cue", nil, nil)
		if err == nil {
			t.Errorf("expected error for invalid CUE, got nil")
		}
	})

	t.Run("override precision", func(t *testing.T) {
		user := `
commodities: {
	BRL: { precision: 2 }
}
`
		cfg, err := Load([]byte(user), "test.cue", nil, nil)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		precision := cfg.Value.LookupPath(cue.ParsePath("commodities.BRL.precision"))
		v, err := precision.Int64()
		if err != nil {
			t.Fatalf("failed to get precision as int: %v", err)
		}
		if v != 2 {
			t.Errorf("expected precision 2, got %d", v)
		}
		usd := cfg.Value.FillPath(cue.ParsePath("commodities.USD"), map[string]any{})
		usdPrecision := usd.LookupPath(cue.ParsePath("commodities.USD.precision"))
		v, err = usdPrecision.Int64()
		if err != nil {
			t.Fatalf("failed to get default precision for USD: %v", err)
		}
		if v != 5 {
			t.Errorf("expected default precision 5 for USD, got %d", v)
		}
	})

	t.Run("ledger names come from discovery inject", func(t *testing.T) {
		discovered := []Ledger{
			{Name: "personal", Main: "/proj/personal/main.beancount"},
			{Name: "acme", Main: "/proj/acme/main.beancount"},
		}
		good := `
links: [{
	name: "ok"
	from: {ledger: "acme", account: "Equity:X"}
	to:   {ledger: "personal", account: "Income:Y"}
}]
`
		cfg, err := Load([]byte(good), "test.cue", discovered, nil)
		if err != nil {
			t.Fatalf("good links: %v", err)
		}
		main := cfg.Value.LookupPath(cue.ParsePath("ledgers.acme.main"))
		s, err := main.String()
		if err != nil || s != "/proj/acme/main.beancount" {
			t.Fatalf("acme.main=%q err=%v", s, err)
		}

		badName := `
links: [{
	name: "bad"
	from: {ledger: "nope", account: "A"}
	to:   {ledger: "personal", account: "B"}
}]
`
		if _, err := Load([]byte(badName), "test.cue", discovered, nil); err == nil {
			t.Fatal("expected error for unknown ledger name in links")
		}
	})

	t.Run("user cannot invent ledger keys", func(t *testing.T) {
		discovered := []Ledger{{Name: "personal", Main: "/p/main.beancount"}}
		// closed inject should reject extra keys from user
		user := `
ledgers: {
	extra: {name: "extra", main: "/x"}
}
`
		if _, err := Load([]byte(user), "test.cue", discovered, nil); err == nil {
			t.Fatal("expected error when user adds ledger not on disk")
		}
	})

	t.Run("price pairs inject closed inventory", func(t *testing.T) {
		pairs := []PricePair{
			{Base: "USD", Quote: "BRL"},
			{Base: "B3_PETR4", Quote: "BRL"},
		}
		cfg, err := Load([]byte("{}"), "test.cue", nil, pairs)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		base := cfg.Value.LookupPath(cue.ParsePath(`price_pairs."USD|BRL".base`))
		s, err := base.String()
		if err != nil || s != "USD" {
			t.Fatalf("USD|BRL.base=%q err=%v", s, err)
		}
		// closed: user cannot invent pairs not in inject
		user := `
price_pairs: {
	"FAKE|BRL": {base: "FAKE", quote: "BRL"}
}
`
		if _, err := Load([]byte(user), "test.cue", nil, pairs); err == nil {
			t.Fatal("expected error inventing price pair")
		}
		// user overlay on existing pair is ok
		overlay := `
price_pairs: {
	"USD|BRL": {source: "BCB"}
}
`
		cfg2, err := Load([]byte(overlay), "test.cue", nil, pairs)
		if err != nil {
			t.Fatalf("overlay: %v", err)
		}
		src := cfg2.Value.LookupPath(cue.ParsePath(`price_pairs."USD|BRL".source`))
		ss, err := src.String()
		if err != nil || ss != "BCB" {
			t.Fatalf("source=%q err=%v", ss, err)
		}
	})

	t.Run("example contapila.cue with discovery", func(t *testing.T) {
		b, err := os.ReadFile("../../testdata/example/contapila.cue")
		if err != nil {
			t.Fatal(err)
		}
		discovered := []Ledger{
			{Name: "personal", Main: "/example/personal/main.beancount"},
			{Name: "acme", Main: "/example/acme/main.beancount"},
			{Name: "ong", Main: "/example/ong/main.beancount"},
			{Name: "smuggle", Main: "/example/smuggle/main.beancount"},
		}
		if _, err := Load(b, "contapila.cue", discovered, nil); err != nil {
			t.Fatalf("example config: %v", err)
		}
	})
}

func TestProjectJournals(t *testing.T) {
	t.Run("prelude defaults", func(t *testing.T) {
		cfg, err := Load([]byte("{}"), "test.cue", nil, nil)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		journals, err := ProjectJournals(cfg.Value)
		if err != nil {
			t.Fatalf("ProjectJournals: %v", err)
		}
		if len(journals) != 2 {
			t.Fatalf("expected 2 default journals, got %d: %+v", len(journals), journals)
		}
		if journals[0].Path != "prices.beancount" || journals[0].Role != "prices" {
			t.Errorf("journals[0]=%+v", journals[0])
		}
		if journals[1].Path != "indexes.beancount" || journals[1].Role != "stream" {
			t.Errorf("journals[1]=%+v", journals[1])
		}
	})

	t.Run("non-list project_journals is an error", func(t *testing.T) {
		// Bypass Load schema so we can exercise List() failure directly.
		ctx := cuecontext.New()
		v := ctx.CompileString(`project_journals: 42`)
		if err := v.Err(); err != nil {
			t.Fatalf("compile: %v", err)
		}
		_, err := ProjectJournals(v)
		if err == nil {
			t.Fatal("expected error for non-list project_journals")
		}
		if !strings.Contains(err.Error(), "project_journals") {
			t.Errorf("error should mention project_journals: %v", err)
		}
	})

	t.Run("missing field is empty", func(t *testing.T) {
		ctx := cuecontext.New()
		v := ctx.CompileString(`other: true`)
		journals, err := ProjectJournals(v)
		if err != nil {
			t.Fatalf("ProjectJournals: %v", err)
		}
		if journals != nil {
			t.Fatalf("expected nil journals, got %+v", journals)
		}
	})

	t.Run("wrong-type path is an error", func(t *testing.T) {
		// Bypass Load schema so we can exercise String() type errors directly.
		ctx := cuecontext.New()
		v := ctx.CompileString(`project_journals: [{path: 42, role: "prices", missing: "warn"}]`)
		if err := v.Err(); err != nil {
			t.Fatalf("compile: %v", err)
		}
		_, err := ProjectJournals(v)
		if err == nil {
			t.Fatal("expected error for non-string path")
		}
		if !strings.Contains(err.Error(), "project_journals.path") {
			t.Errorf("error should mention project_journals.path: %v", err)
		}
	})

	t.Run("wrong-type role is an error", func(t *testing.T) {
		ctx := cuecontext.New()
		v := ctx.CompileString(`project_journals: [{path: "x.beancount", role: 99, missing: "warn"}]`)
		if err := v.Err(); err != nil {
			t.Fatalf("compile: %v", err)
		}
		_, err := ProjectJournals(v)
		if err == nil {
			t.Fatal("expected error for non-string role")
		}
		if !strings.Contains(err.Error(), "project_journals.role") {
			t.Errorf("error should mention project_journals.role: %v", err)
		}
	})

	t.Run("wrong-type missing is an error", func(t *testing.T) {
		ctx := cuecontext.New()
		v := ctx.CompileString(`project_journals: [{path: "x.beancount", role: "prices", missing: true}]`)
		if err := v.Err(); err != nil {
			t.Fatalf("compile: %v", err)
		}
		_, err := ProjectJournals(v)
		if err == nil {
			t.Fatal("expected error for non-string missing")
		}
		if !strings.Contains(err.Error(), "project_journals.missing") {
			t.Errorf("error should mention project_journals.missing: %v", err)
		}
	})

	t.Run("absent optional fields keep skip/default behavior", func(t *testing.T) {
		ctx := cuecontext.New()
		// No role → skip entry; empty path → skip; path+role only → missing defaults to ignore.
		v := ctx.CompileString(`project_journals: [
			{path: "skip-no-role.beancount"},
			{path: "", role: "prices"},
			{path: "ok.beancount", role: "stream"},
		]`)
		if err := v.Err(); err != nil {
			t.Fatalf("compile: %v", err)
		}
		journals, err := ProjectJournals(v)
		if err != nil {
			t.Fatalf("ProjectJournals: %v", err)
		}
		if len(journals) != 1 {
			t.Fatalf("expected 1 journal, got %d: %+v", len(journals), journals)
		}
		if journals[0].Path != "ok.beancount" || journals[0].Role != "stream" || journals[0].Missing != "ignore" {
			t.Errorf("journals[0]=%+v", journals[0])
		}
	})
}

func TestEncodeLedgersCUE_content(t *testing.T) {
	src := encodeLedgersCUE([]Ledger{
		{Name: "b", Main: "/b"},
		{Name: "a", Main: "/a"},
	})
	if !strings.Contains(src, "ledgers: close({") {
		t.Fatalf("expected close(: %s", src)
	}
	ia, ib := strings.Index(src, "	a:"), strings.Index(src, "	b:")
	if ia < 0 || ib < 0 || ia > ib {
		t.Fatalf("expected sorted keys a then b: %s", src)
	}
}

func TestEncodePricePairsCUE_content(t *testing.T) {
	src := encodePricePairsCUE([]PricePair{
		{Base: "USD", Quote: "BRL"},
		{Base: "B3_PETR4", Quote: "BRL"},
	})
	if !strings.Contains(src, "price_pairs: close({") {
		t.Fatalf("expected close: %s", src)
	}
	if !strings.Contains(src, `"B3_PETR4|BRL"`) || !strings.Contains(src, `"USD|BRL"`) {
		t.Fatalf("expected pair keys: %s", src)
	}
	// sorted: B3 before USD
	ib, iu := strings.Index(src, `"B3_PETR4|BRL"`), strings.Index(src, `"USD|BRL"`)
	if ib < 0 || iu < 0 || ib > iu {
		t.Fatalf("expected B3 before USD: %s", src)
	}
}
