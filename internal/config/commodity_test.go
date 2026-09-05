package config

import (
	"math/big"
	"testing"
)

func TestHalfULP(t *testing.T) {
	// precision 2 → 0.005
	got := HalfULP(2)
	want := big.NewRat(5, 1000)
	if got.Cmp(want) != 0 {
		t.Fatalf("got %s want %s", got.FloatString(6), want.FloatString(6))
	}
	// precision 5 → 5e-6
	got = HalfULP(5)
	want = big.NewRat(5, 1000000)
	if got.Cmp(want) != 0 {
		t.Fatalf("got %s want %s", got.FloatString(10), want.FloatString(10))
	}
}

func TestCommodityPoliciesToleranceOverride(t *testing.T) {
	user := []byte(`
commodities: {
  BRL: { precision: 2, tolerance: 0.01 }
}
`)
	cfg, err := Load(user, "t.cue", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := CommodityPolicies(cfg.Value)
	p := PolicyFor(m, "BRL")
	if p.Precision != 2 {
		t.Fatalf("prec=%d", p.Precision)
	}
	want := big.NewRat(1, 100)
	if p.Tolerance.Cmp(want) != 0 {
		t.Fatalf("tol=%s want %s", p.Tolerance.FloatString(4), want.FloatString(4))
	}
	// unknown commodity → default
	d := PolicyFor(m, "XYZ")
	if d.Precision != 5 {
		t.Fatalf("default prec=%d", d.Precision)
	}
}
