package web

import (
	"context"
	"errors"
	"sync"

	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/prices"
	"github.com/lucasew/contapila-go/pkg/project"
)

// ErrSessionNil is returned when Session methods are called on a nil receiver.
var ErrSessionNil = errors.New("web: session is nil")

// Session memoizes project and ledger loads for one lifetime:
//
//   - Live web: middleware attaches a new Session per HTTP request so the first
//     handler call warms disks/books and later calls on that request reuse them;
//     the next request gets a new Session and recalculates (F5 freshness).
//   - contapila build: one Session for expand + every page fetch so OpenProject
//     and OpenLedger run once per name for the whole site.
//
// All ingestion should go through Project / Ledger — not engine.Open* ad hoc.
type Session struct {
	Root string

	onceProject sync.Once
	project     *project.Project
	prices      *prices.DB
	projectErr  error

	ledgerMu sync.Mutex
	ledgers  map[string]*engine.Ledger
}

// NewSession returns a cold session for root (lazy OpenProject on first Project()).
func NewSession(root string) *Session {
	return &Session{Root: root}
}

type sessionCtxKey struct{}

// withSession stores s on ctx. s must be non-nil for handlers that need it.
func withSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, sessionCtxKey{}, s)
}

// sessionFrom returns the Session attached to ctx, or nil.
func sessionFrom(ctx context.Context) *Session {
	s, _ := ctx.Value(sessionCtxKey{}).(*Session)
	return s
}

// Project opens the project and prices once (sync.Once), then returns them.
func (s *Session) Project() (*project.Project, *prices.DB, error) {
	if s == nil {
		return nil, nil, ErrSessionNil
	}
	s.onceProject.Do(func() {
		if s.Root == "" {
			s.projectErr = ErrProjectRootRequired
			return
		}
		s.project, s.prices, _, s.projectErr = engine.OpenProject(s.Root)
	})
	return s.project, s.prices, s.projectErr
}

// Ledger returns a booked ledger by directory name, opening it once per name.
func (s *Session) Ledger(name string) (*engine.Ledger, error) {
	if s == nil {
		return nil, ErrSessionNil
	}
	proj, pdb, err := s.Project()
	if err != nil {
		return nil, err
	}

	s.ledgerMu.Lock()
	if l, ok := s.ledgers[name]; ok {
		s.ledgerMu.Unlock()
		return l, nil
	}
	s.ledgerMu.Unlock()

	l, err := engine.OpenLedger(proj, pdb, name)
	if err != nil {
		return nil, err
	}

	s.ledgerMu.Lock()
	defer s.ledgerMu.Unlock()
	if existing, ok := s.ledgers[name]; ok {
		return existing, nil
	}
	if s.ledgers == nil {
		s.ledgers = make(map[string]*engine.Ledger)
	}
	s.ledgers[name] = l
	return l, nil
}

// LedgerNames returns sorted ledger directory names after Project() succeeds.
func (s *Session) LedgerNames() ([]string, error) {
	p, _, err := s.Project()
	if err != nil {
		return nil, err
	}
	return engine.LedgerNames(p), nil
}
