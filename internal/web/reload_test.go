package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/engine"
)

// F5 must see disk changes: project discovery, CUE config, and prices are reloaded
// on every request (not pinned at Listen/New time).
func TestRequestReloadsProjectConfigAndPrices(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("contapila.cue", `{
	commodities: {
		USD: { name: "US Dollar", precision: 2 }
	}
}
`)
	mustWrite("personal/main.beancount", `option "operating_currency" "BRL"
2024-01-01 open Assets:Cash BRL
2024-01-01 * "seed"
  Assets:Cash  100.00 BRL
  Equity:Opening
`)
	mustWrite("prices.beancount", `2024-01-01 price USD 5.00 BRL
`)

	p, pdb, _, err := engine.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()

	// Baseline: one ledger, CUE name on commodity page, price 5.00.
	// Match ledger links specifically — layout HTML also contains "sidebar"/"aside".
	const personalLink = `href="/l/personal/check"`
	const extraLink = `href="/l/extra/check"`
	{
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("index status %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, personalLink) {
			t.Fatalf("index missing personal ledger: %s", truncate(body, 200))
		}
		if strings.Contains(body, extraLink) {
			t.Fatalf("index should not list extra yet: %s", truncate(body, 200))
		}
	}
	{
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/commodity/USD", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("commodity status %d body=%s", rr.Code, truncate(rr.Body.String(), 300))
		}
		body := rr.Body.String()
		if !strings.Contains(body, "US Dollar") {
			t.Fatalf("commodity page missing CUE name before edit: %s", truncate(body, 400))
		}
		if !strings.Contains(body, "5.00") && !strings.Contains(body, "5") {
			t.Fatalf("commodity page missing price 5 before edit: %s", truncate(body, 400))
		}
	}

	// Mutate config, prices, and ledger set on disk without restarting the server.
	mustWrite("contapila.cue", `{
	commodities: {
		USD: { name: "Fresh Dollar", precision: 2 }
	}
}
`)
	mustWrite("prices.beancount", `2024-01-01 price USD 9.99 BRL
`)
	mustWrite("extra/main.beancount", `option "operating_currency" "BRL"
2024-01-01 open Assets:Cash BRL
`)

	// Fresh request must reflect all three changes.
	{
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("index after edit status %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, extraLink) {
			t.Fatalf("index did not reload new ledger: %s", truncate(body, 300))
		}
	}
	{
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/commodity/USD", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("commodity after edit status %d body=%s", rr.Code, truncate(rr.Body.String(), 300))
		}
		body := rr.Body.String()
		if !strings.Contains(body, "Fresh Dollar") {
			t.Fatalf("commodity page did not reload CUE name: %s", truncate(body, 400))
		}
		if strings.Contains(body, "US Dollar") {
			t.Fatalf("commodity page still shows stale CUE name: %s", truncate(body, 400))
		}
		if !strings.Contains(body, "9.99") {
			t.Fatalf("commodity page did not reload prices: %s", truncate(body, 400))
		}
	}
}
