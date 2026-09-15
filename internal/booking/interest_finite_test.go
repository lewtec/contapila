package booking

import (
	"math"
	"math/big"
	"testing"
)

func TestParseInterestRateRejectsAbsurdPlus(t *testing.T) {
	// Extremely large %aa can yield a non-finite daily factor after float conversion.
	if ir, ok := ParseInterestRate("1e308%aa"); ok {
		t.Fatalf("ParseInterestRate accepted absurd rate: %+v", ir)
	}
}

func TestParsePlusDailyFiniteOnly(t *testing.T) {
	r, ok := parsePlusDaily("10%aa")
	if !ok || r == nil {
		t.Fatal("expected 10%aa to parse")
	}
	if r.Sign() <= 0 {
		t.Fatalf("daily factor should be positive, got %s", r.FloatString(12))
	}
	// Non-finite path: SetFloat64(nil) contract
	if got := new(big.Rat).SetFloat64(math.Inf(1)); got != nil {
		t.Fatal("expected SetFloat64(+Inf) == nil")
	}
	if got := new(big.Rat).SetFloat64(math.NaN()); got != nil {
		t.Fatal("expected SetFloat64(NaN) == nil")
	}
}
