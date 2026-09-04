package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/pkg/project"
)

func TestCheckPageShowsDocsFolderDateDiags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "contapila.cue"), []byte("//\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := filepath.Join(root, "personal")
	docDir := filepath.Join(ledger, "docs", "by-account", "Assets", "Cash")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledger, "main.beancount"), []byte(`
option "operating_currency" "BRL"
2020-01-01 open Assets:Cash BRL
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docDir, "20241301_bad.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := project.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/check", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Check failed") {
		t.Fatalf("want Check failed:\n%s", body)
	}
	if !strings.Contains(body, "20241301") || !strings.Contains(body, "not a valid calendar date") {
		t.Fatalf("want docs date error on check page:\n%s", body)
	}
}
