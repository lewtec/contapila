package engine

import (
	"math/big"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestNetWorthTableVsChartLastPoint(t *testing.T) {
	_, f, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(f), "..", "..", "testdata", "example")
	p, pdb, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(p, pdb, "personal")
	if err != nil {
		t.Fatal(err)
	}
	// Table default as-of is far future; chart terminal is "today". Compare both at today.
	today := time.Now().UTC()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	_, tableTot, err := l.NetWorth(today)
	if err != nil {
		t.Fatal(err)
	}
	pts, err := l.NetWorthSeries(time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) == 0 {
		t.Fatal("no series points")
	}
	last := pts[len(pts)-1]
	t.Logf("table@today=%s lastChart=%s on %s n=%d",
		tableTot.FloatString(2), last.Value.FloatString(2), last.Date.Format("2006-01-02"), len(pts))
	if last.Date.Before(today.AddDate(0, 0, -1)) {
		// allow same calendar day; fail if terminal is far in the past
		t.Errorf("chart last sample %s should be ~today %s", last.Date.Format("2006-01-02"), today.Format("2006-01-02"))
	}
	diff := new(big.Rat).Sub(tableTot, last.Value)
	tol := big.NewRat(1, 100) // 0.01
	if new(big.Rat).Abs(diff).Cmp(tol) > 0 {
		t.Errorf("table NW and chart last point disagree by %s (table=%s chart=%s)",
			diff.FloatString(2), tableTot.FloatString(2), last.Value.FloatString(2))
	}
}
