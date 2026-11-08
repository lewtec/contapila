package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lucasew/contapila-go/internal/engine"
)

func TestSessionMemoizesProjectAndLedger(t *testing.T) {
	sess := NewSession(exampleRoot(t))
	p1, pdb1, err := sess.Project(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	p2, pdb2, err := sess.Project(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if p1 != p2 || pdb1 != pdb2 {
		t.Fatal("Project() should return the same pointers after Once")
	}

	l1, err := sess.Ledger(t.Context(), "personal")
	if err != nil {
		t.Fatal(err)
	}
	l2, err := sess.Ledger(t.Context(), "personal")
	if err != nil {
		t.Fatal(err)
	}
	if l1 != l2 {
		t.Fatal("Ledger() should memoize by name")
	}
	if l1.Name != "personal" {
		t.Fatalf("name=%q", l1.Name)
	}
}

func TestSessionNil(t *testing.T) {
	var sess *Session
	if _, _, err := sess.Project(t.Context()); err != ErrSessionNil {
		t.Fatalf("Project: %v", err)
	}
	if _, err := sess.Ledger(t.Context(), "x"); err != ErrSessionNil {
		t.Fatalf("Ledger: %v", err)
	}
}

func TestLiveMiddlewareNewSessionPerRequest(t *testing.T) {
	root := exampleRoot(t)
	p, pdb, _, err := engine.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()

	// Two separate requests both succeed (each gets a cold Session).
	for i := 0; i < 2; i++ {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/l/personal/check", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: status %d", i, rr.Code)
		}
	}
}

func TestBuildSessionSharedOnRequest(t *testing.T) {
	root := exampleRoot(t)
	sess := NewSession(root)
	p, pdb, err := sess.Project(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// Warm personal once.
	if _, err := sess.Ledger(t.Context(), "personal"); err != nil {
		t.Fatal(err)
	}
	s, err := New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()

	// Same session on two paths — should not error; ledger pointer stable.
	for _, path := range []string{"/l/personal/check", "/l/personal/balances"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(withSession(req.Context(), sess))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s: status %d", path, rr.Code)
		}
	}
	l, err := sess.Ledger(t.Context(), "personal")
	if err != nil || l == nil {
		t.Fatalf("ledger after requests: %v", err)
	}
}
