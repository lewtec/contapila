package web

import "testing"

func TestPublicHrefsLiveAndStatic(t *testing.T) {
	live := &Session{}
	if got := HomeHref(live); got != "/" {
		t.Fatalf("HomeHref live: %q", got)
	}
	if got := LedgerHref(live, "personal", "check", "2024"); got != "/l/personal/check?time=2024" {
		t.Fatalf("LedgerHref live: %q", got)
	}
	if got := AccountHref(live, "personal", "Assets:Cash", ""); got != "/l/personal/account/Assets:Cash" {
		t.Fatalf("AccountHref live: %q", got)
	}
	if got := CommodityHref(live, "personal", "BRL", "2024-01"); got != "/l/personal/commodity/BRL?time=2024-01" {
		t.Fatalf("CommodityHref live: %q", got)
	}
	if got := LedgerRootHref(live, "personal"); got != "/l/personal/" {
		t.Fatalf("LedgerRootHref live: %q", got)
	}

	st := &Session{Static: true}
	if got := HomeHref(st); got != "/index.html" {
		t.Fatalf("HomeHref static: %q", got)
	}
	if got := LedgerHref(st, "personal", "check", "2024"); got != "/l/personal/check.html" {
		t.Fatalf("LedgerHref static: %q", got)
	}
	if got := AccountHref(st, "personal", "Assets:Cash", "x"); got != "/l/personal/account/Assets:Cash.html" {
		t.Fatalf("AccountHref static: %q", got)
	}
	if got := CommodityHref(st, "personal", "USD", ""); got != "/l/personal/commodity/USD.html" {
		t.Fatalf("CommodityHref static: %q", got)
	}
	if got := LedgerRootHref(st, "acme"); got != "/l/acme/index.html" {
		t.Fatalf("LedgerRootHref static: %q", got)
	}
}
