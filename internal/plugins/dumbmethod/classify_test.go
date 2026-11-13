package dumbmethod

import (
	"errors"
	"math/big"
	"testing"

	"github.com/lucasew/contapila-go/internal/config"
)

func testConfig(t *testing.T, user string) *config.Config {
	t.Helper()
	cfg, err := config.Load([]byte(user), "contapila.cue", nil, nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

func TestClassifyUnifiesHoldingsAndRules(t *testing.T) {
	cfg := testConfig(t, `
plugins: dumb_method: {
	enabled: true
	settings: {
		classes: {
			acoes: {target: 60}
			fiis: {target: 40}
		}
		#Classify: {
			holdings: [...#Holding]
			classified: [for h in holdings {
				h & {
					if h.account =~ "^Assets:BR:B3:Acao$" {
						class: "acoes"
					}
					if h.commodity == "B3_PETR4" {
						force: 8
						id: "PETR4"
					}
					if h.account == "Assets:BR:B3:FII" {
						class: "fiis"
						force: 5
						id: "MXRF11"
					}
				}
			}]
		}
	}
}
`)
	raw := []rawHolding{
		{Index: 0, Account: "Assets:BR:B3:Acao", Commodity: "B3_PETR4", Units: big.NewRat(10, 1), Value: big.NewRat(300, 1)},
		{Index: 1, Account: "Assets:BR:B3:FII", Commodity: "B3_MXRF11", Units: big.NewRat(100, 1), Value: big.NewRat(1000, 1)},
		{Index: 2, Account: "Assets:Cash:Carteira", Commodity: "BRL", Units: big.NewRat(50, 1), Value: big.NewRat(50, 1)},
	}
	got, warns, err := classify(cfg.Value, raw)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("warnings: %v", warns)
	}
	if c := got[0]; c.Class != "acoes" || !c.HasForce || c.Force != 8 || c.ID != "PETR4" {
		t.Errorf("holding 0: %+v", c)
	}
	if c := got[1]; c.Class != "fiis" || !c.HasForce || c.Force != 5 || c.ID != "MXRF11" {
		t.Errorf("holding 1: %+v", c)
	}
	if c := got[2]; c.Class != "" || c.HasForce || c.ID != "" {
		t.Errorf("holding 2 should be unclassified: %+v", c)
	}

	classes, cwarns := classTargets(cfg.Value)
	if len(cwarns) != 0 {
		t.Fatalf("class warnings: %v", cwarns)
	}
	if len(classes) != 2 {
		t.Fatalf("classes: %+v", classes)
	}
	// Sorted by target descending.
	if classes[0].ID != "acoes" || classes[0].Target.Cmp(big.NewRat(60, 1)) != 0 {
		t.Errorf("classes[0]: %+v", classes[0])
	}
}

func TestClassifyWithoutRules(t *testing.T) {
	cfg := testConfig(t, `
plugins: dumb_method: {
	enabled: true
	settings: classes: acoes: {target: 100}
}
`)
	_, _, err := classify(cfg.Value, nil)
	if !errors.Is(err, ErrNoClassify) {
		t.Fatalf("err=%v want ErrNoClassify", err)
	}
}

func TestTargetsOverHundredRejected(t *testing.T) {
	_, err := config.Load([]byte(`
plugins: dumb_method: {
	enabled: true
	settings: classes: {
		a: {target: 60}
		b: {target: 50}
	}
}
`), "contapila.cue", nil, nil)
	if err == nil {
		t.Fatal("expected target sum > 100 to fail unification")
	}
}

func TestWeightSumMustMatchTargets(t *testing.T) {
	t.Run("defaults to 100", func(t *testing.T) {
		cfg := testConfig(t, `
plugins: dumb_method: {
	enabled: true
	settings: classes: {
		a: {target: 60}
		b: {target: 40}
	}
}
`)
		classes, warns := classTargets(cfg.Value)
		if len(warns) != 0 {
			t.Fatalf("warnings: %v", warns)
		}
		if len(classes) != 2 {
			t.Fatalf("classes: %+v", classes)
		}
	})

	t.Run("default rejects partial sum", func(t *testing.T) {
		_, err := config.Load([]byte(`
plugins: dumb_method: {
	enabled: true
	settings: classes: a: {target: 50}
}
`), "contapila.cue", nil, nil)
		if err == nil {
			t.Fatal("expected target sum 50 to fail the default 100 weight_sum")
		}
	})

	t.Run("explicit lower sum", func(t *testing.T) {
		cfg := testConfig(t, `
plugins: dumb_method: {
	enabled: true
	settings: {
		weight_sum: 50
		classes: {
			a: {target: 30}
			b: {target: 20}
		}
	}
}
`)
		classes, warns := classTargets(cfg.Value)
		if len(warns) != 0 {
			t.Fatalf("warnings: %v", warns)
		}
		if len(classes) != 2 {
			t.Fatalf("classes: %+v", classes)
		}
	})

	t.Run("explicit mismatch rejected", func(t *testing.T) {
		_, err := config.Load([]byte(`
plugins: dumb_method: {
	enabled: true
	settings: {
		weight_sum: 60
		classes: {
			a: {target: 30}
			b: {target: 20}
		}
	}
}
`), "contapila.cue", nil, nil)
		if err == nil {
			t.Fatal("expected target sum 50 to fail weight_sum 60")
		}
	})

	t.Run("empty classes stay valid", func(t *testing.T) {
		testConfig(t, `
plugins: dumb_method: {
	enabled: true
	settings: classes: {}
}
`)
	})
}
