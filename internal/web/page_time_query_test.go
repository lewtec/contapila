package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lucasew/contapila-go/internal/engine"
)

func TestParsePageTimeQuery(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	t.Run("time wins over from/to", func(t *testing.T) {
		q := url.Values{"time": {"2024"}, "from": {"2023"}, "to": {"2023"}}
		got, err := parsePageTimeQuery(q, now, true, true)
		if err != nil {
			t.Fatal(err)
		}
		if got.Time != "2024" {
			t.Fatalf("Time=%q want 2024", got.Time)
		}
	})

	t.Run("from/to composition when fromTo enabled", func(t *testing.T) {
		q := url.Values{"from": {"2024-01"}, "to": {"2024-03"}}
		got, err := parsePageTimeQuery(q, now, true, true)
		if err != nil {
			t.Fatal(err)
		}
		if got.Time != "2024-01 - 2024-03" {
			t.Fatalf("Time=%q", got.Time)
		}
	})

	t.Run("from/to ignored when fromTo disabled", func(t *testing.T) {
		// Helper flag still works; page handlers pass fromTo=true.
		q := url.Values{"from": {"2024-01"}, "to": {"2024-03"}}
		got, err := parsePageTimeQuery(q, now, false, false)
		if err != nil {
			t.Fatal(err)
		}
		if got.Time != "" {
			t.Fatalf("Time=%q want empty", got.Time)
		}
		if !got.AsOf.Equal(engine.AsOfLatest) {
			t.Fatalf("AsOf=%v want AsOfLatest", got.AsOf)
		}
		if got.AsOfStr != "" {
			t.Fatalf("AsOfStr=%q want empty", got.AsOfStr)
		}
	})

	t.Run("from only", func(t *testing.T) {
		q := url.Values{"from": {"2024-01"}}
		got, err := parsePageTimeQuery(q, now, true, true)
		if err != nil {
			t.Fatal(err)
		}
		if got.Time != "2024-01" {
			t.Fatalf("Time=%q", got.Time)
		}
	})

	t.Run("to only", func(t *testing.T) {
		q := url.Values{"to": {"2024-03"}}
		got, err := parsePageTimeQuery(q, now, true, true)
		if err != nil {
			t.Fatal(err)
		}
		if got.Time != "2024-03" {
			t.Fatalf("Time=%q", got.Time)
		}
	})

	t.Run("as-of from period end", func(t *testing.T) {
		q := url.Values{"time": {"2024-03"}}
		got, err := parsePageTimeQuery(q, now, true, true)
		if err != nil {
			t.Fatal(err)
		}
		wantEnd := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)
		if !got.AsOf.Equal(wantEnd) {
			t.Fatalf("AsOf=%v want %v", got.AsOf, wantEnd)
		}
		if got.AsOfStr != "2024-03-31" {
			t.Fatalf("AsOfStr=%q", got.AsOfStr)
		}
	})

	t.Run("explicit as-of wins", func(t *testing.T) {
		q := url.Values{"time": {"2024-03"}, "as-of": {"2024-02-10"}}
		got, err := parsePageTimeQuery(q, now, true, true)
		if err != nil {
			t.Fatal(err)
		}
		want := time.Date(2024, 2, 10, 0, 0, 0, 0, time.UTC)
		if !got.AsOf.Equal(want) {
			t.Fatalf("AsOf=%v want %v", got.AsOf, want)
		}
		if got.AsOfStr != "2024-02-10" {
			t.Fatalf("AsOfStr=%q", got.AsOfStr)
		}
	})

	t.Run("explicit as-of ignored when disabled", func(t *testing.T) {
		// Helper flag still works; page handlers pass explicitAsOf=true.
		q := url.Values{"time": {"2024-03"}, "as-of": {"2024-02-10"}}
		got, err := parsePageTimeQuery(q, now, false, false)
		if err != nil {
			t.Fatal(err)
		}
		wantEnd := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)
		if !got.AsOf.Equal(wantEnd) {
			t.Fatalf("AsOf=%v want period end %v", got.AsOf, wantEnd)
		}
	})

	t.Run("invalid as-of", func(t *testing.T) {
		q := url.Values{"as-of": {"not-a-date"}}
		_, err := parsePageTimeQuery(q, now, true, true)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty filter uses AsOfLatest", func(t *testing.T) {
		got, err := parsePageTimeQuery(url.Values{}, now, true, true)
		if err != nil {
			t.Fatal(err)
		}
		if !got.AsOf.Equal(engine.AsOfLatest) {
			t.Fatalf("AsOf=%v want AsOfLatest", got.AsOf)
		}
		if got.AsOfStr != "" {
			t.Fatalf("AsOfStr=%q", got.AsOfStr)
		}
	})
}

// Account, commodity, and ledger pages share the full time query surface
// (from/to composition + explicit as-of with 400 on invalid).
func TestPageTimeQueryHandlersParity(t *testing.T) {
	s := testWebServer(t)
	paths := []string{
		"/l/personal/balances",
		"/l/personal/account/Assets:BR:Alfa:ContaCorrente",
		"/l/personal/commodity/USD",
	}
	for _, path := range paths {
		t.Run("invalid as-of 400 "+path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path+"?as-of=not-a-date", nil)
			rr := httptest.NewRecorder()
			s.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status %d want 400 body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "as-of") {
				t.Fatalf("body missing as-of error: %s", rr.Body.String())
			}
		})
		t.Run("from/to and as-of ok "+path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path+"?from=2024-01&to=2024-03&as-of=2024-02-10", nil)
			rr := httptest.NewRecorder()
			s.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status %d body=%s", rr.Code, rr.Body.String())
			}
			// as-of should surface in the page when set
			if !strings.Contains(rr.Body.String(), "2024-02-10") {
				t.Fatalf("response missing as-of date; body head=%q", truncate(rr.Body.String(), 200))
			}
		})
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
