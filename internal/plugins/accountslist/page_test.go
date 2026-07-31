package accountslist_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/plugins/accountslist"
	"github.com/lucasew/contapila-go/internal/web"
)

func TestAccountsPluginRegistered(t *testing.T) {
	// Bind middlemen then look up page in package-local plugin registry.
	web.BindPluginPages()
	found := false
	// ReportPages includes plugin pages after bind.
	for _, id := range web.ReportPages() {
		if id == accountslist.PageID {
			found = true
		}
	}
	if !found {
		t.Fatal("accounts page not in ReportPages after BindPluginPages")
	}
	_ = accountslist.PluginKey
}

func TestAccountsPageLive(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "example"))
	if err != nil {
		t.Fatal(err)
	}
	p, pdb, _, err := engine.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := web.New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/accounts", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rr.Code, truncate(rr.Body.String(), 200))
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Accounts") {
		t.Fatal("missing Accounts label")
	}
	// Example personal ledger has open accounts.
	if !strings.Contains(body, "Assets:") && !strings.Contains(body, "Equity:") {
		t.Fatalf("expected account names in body snippet: %s", truncate(body, 400))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
