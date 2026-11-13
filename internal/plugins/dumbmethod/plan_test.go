package dumbmethod

import (
	"math/big"
	"strings"
	"testing"
)

func mustRat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic("bad rat " + s)
	}
	return r
}

func rat(t *testing.T, s string) *big.Rat {
	t.Helper()
	return mustRat(s)
}

func holding(class, id, commodity string, value, price string, force float64) HoldingInput {
	return HoldingInput{
		Class:      class,
		AssetID:    id,
		AssetLabel: id,
		Commodity:  commodity,
		Account:    "Assets:" + id,
		Value:      mustRat(value),
		Force:      force,
		HasForce:   force != 0,
		Price:      mustRat(price),
		Priced:     price != "",
		Precision:  2,
	}
}

func TestComputeScalesClassGaps(t *testing.T) {
	classes := []ClassInput{
		{ID: "acoes_br", Target: rat(t, "30")},
		{ID: "fiis", Target: rat(t, "15")},
		{ID: "exterior", Target: rat(t, "20")},
		{ID: "renda_fixa", Target: rat(t, "25")},
		{ID: "cripto", Target: rat(t, "10")},
	}
	holdings := []HoldingInput{
		holding("acoes_br", "PETR4", "B3_PETR4", "4000", "1", 8),
		holding("fiis", "MXRF11", "B3_MXRF11", "1000", "1", 8),
		holding("exterior", "SPDW", "SPDW", "3000", "1", 9),
		holding("renda_fixa", "CDB", "BRL", "1500", "1", 5),
		holding("cripto", "BTC", "BTC", "500", "1", 10),
	}

	p := Compute(rat(t, "1000"), classes, holdings)

	if p.Total.Cmp(rat(t, "10000")) != 0 {
		t.Fatalf("total %s want 10000", p.Total)
	}
	if p.NewTotal.Cmp(rat(t, "11000")) != 0 {
		t.Fatalf("new total %s want 11000", p.NewTotal)
	}
	wantBudget := map[string]string{
		"acoes_br":   "0",
		"fiis":       "260",
		"exterior":   "0",
		"renda_fixa": "500",
		"cripto":     "240",
	}
	for _, cp := range p.Classes {
		want := rat(t, wantBudget[cp.ID])
		if cp.Budget.Cmp(want) != 0 {
			t.Errorf("%s budget %s want %s", cp.ID, cp.Budget, want)
		}
	}
	if p.Allocated.Cmp(rat(t, "1000")) != 0 {
		t.Errorf("allocated %s want 1000", p.Allocated)
	}
	if p.Uninvested.Sign() != 0 {
		t.Errorf("uninvested %s want 0", p.Uninvested)
	}
}

func TestComputeForceWeightedShares(t *testing.T) {
	classes := []ClassInput{{ID: "fiis", Target: rat(t, "100")}}
	holdings := []HoldingInput{
		holding("fiis", "MXRF11", "B3_MXRF11", "600", "1", 8),
		holding("fiis", "HGLG11", "B3_HGLG11", "400", "1", 2),
	}

	p := Compute(rat(t, "1000"), classes, holdings)
	// Class after = 2000. Force shares: MXRF11 1600, HGLG11 400.
	if p.Allocated.Cmp(rat(t, "1000")) != 0 {
		t.Fatalf("allocated %s want 1000", p.Allocated)
	}
	for _, ap := range p.Assets {
		switch ap.Label {
		case "MXRF11":
			if ap.Buy.Cmp(rat(t, "1000")) != 0 {
				t.Errorf("MXRF11 buy %s want 1000", ap.Buy)
			}
		case "HGLG11":
			if ap.Buy.Sign() != 0 {
				t.Errorf("HGLG11 buy %s want 0", ap.Buy)
			}
		}
	}
}

func TestComputeForceZeroCountsButNeverBuys(t *testing.T) {
	classes := []ClassInput{{ID: "fiis", Target: rat(t, "100")}}
	zero := holding("fiis", "ZERO", "B3_ZERO", "400", "1", 0)
	zero.HasForce = true // explicit force 0 must never buy
	holdings := []HoldingInput{
		zero,
		holding("fiis", "MXRF11", "B3_MXRF11", "600", "1", 10),
	}

	p := Compute(rat(t, "1000"), classes, holdings)
	if p.Total.Cmp(rat(t, "1000")) != 0 {
		t.Fatalf("total %s want 1000 (force-zero still counts)", p.Total)
	}
	for _, ap := range p.Assets {
		switch ap.Label {
		case "ZERO":
			if ap.Buy.Sign() != 0 || !strings.Contains(ap.Status, "force 0") {
				t.Errorf("ZERO: buy=%s status=%q", ap.Buy, ap.Status)
			}
		case "MXRF11":
			if ap.Buy.Cmp(rat(t, "1000")) != 0 {
				t.Errorf("MXRF11 buy %s want 1000", ap.Buy)
			}
		}
	}
}

func TestComputeUnclassifiedExcludedFromAllocation(t *testing.T) {
	classes := []ClassInput{{ID: "acoes", Target: rat(t, "50")}}
	holdings := []HoldingInput{
		holding("acoes", "PETR4", "B3_PETR4", "500", "1", 8),
		holding("", "CASH", "BRL", "500", "1", 0),
	}

	p := Compute(rat(t, "1000"), classes, holdings)
	if p.Total.Cmp(rat(t, "500")) != 0 {
		t.Fatalf("total %s want 500 (classified only)", p.Total)
	}
	if p.NewTotal.Cmp(rat(t, "1500")) != 0 {
		t.Fatalf("new total %s want 1500", p.NewTotal)
	}
	if p.Excluded.Cmp(rat(t, "500")) != 0 {
		t.Fatalf("excluded %s want 500", p.Excluded)
	}
	if len(p.Unclassified) != 1 {
		t.Fatalf("unclassified %d want 1", len(p.Unclassified))
	}
	// 50% of 1500 = 750 desired, 500 current, so 250 to buy. Counting the
	// excluded cash would have made the gap 500 instead.
	if p.Allocated.Cmp(rat(t, "250")) != 0 {
		t.Errorf("allocated %s want 250", p.Allocated)
	}
}

func TestComputeFloorsUnitsAndLeavesRemainder(t *testing.T) {
	classes := []ClassInput{{ID: "acoes", Target: rat(t, "100")}}
	h := holding("acoes", "PETR4", "B3_PETR4", "300", "300", 8)
	h.Precision = 0
	p := Compute(rat(t, "1000"), classes, []HoldingInput{h})

	// Class after = 1300, gap = 1000. At 300/unit only 3 whole units fit.
	if p.Allocated.Cmp(rat(t, "900")) != 0 {
		t.Fatalf("allocated %s want 900", p.Allocated)
	}
	if p.Uninvested.Cmp(rat(t, "100")) != 0 {
		t.Fatalf("uninvested %s want 100", p.Uninvested)
	}
	if got := p.Assets[0].Units; got == nil || got.Cmp(rat(t, "3")) != 0 {
		t.Fatalf("units %v want 3", got)
	}
}

func TestComputeNothingToDoKeepsCash(t *testing.T) {
	classes := []ClassInput{{ID: "acoes", Target: rat(t, "50")}}
	holdings := []HoldingInput{
		holding("acoes", "PETR4", "B3_PETR4", "1000", "1", 8),
	}
	p := Compute(rat(t, "500"), classes, holdings)
	if p.Allocated.Sign() != 0 {
		t.Errorf("allocated %s want 0 (class above target)", p.Allocated)
	}
	if p.Uninvested.Cmp(rat(t, "500")) != 0 {
		t.Errorf("uninvested %s want 500", p.Uninvested)
	}
}

func TestFloorToStep(t *testing.T) {
	cases := []struct {
		in        string
		precision int
		want      string
	}{
		{"1.239", 2, "1.23"},
		{"1.999", 0, "1"},
		{"0.000000019", 8, "0.00000001"},
	}
	for _, tc := range cases {
		got := floorToStep(rat(t, tc.in), tc.precision)
		if got.Cmp(rat(t, tc.want)) != 0 {
			t.Errorf("floor(%s,%d)=%s want %s", tc.in, tc.precision, got, tc.want)
		}
	}
}
