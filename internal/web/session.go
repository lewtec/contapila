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
// All ingestion should go through Handle / Ledger — not engine.OpenProject ad hoc.
// URL helpers (HomeURL, LedgerURL, …) honor Static so build emits .html / index.html
// hrefs natively without post-processing HTML.
type Session struct {
	Root string
	// Static is true for contapila build: link methods use on-disk URL shapes.
	Static bool

	onceOpen sync.Once
	handle   *engine.Handle
	openErr  error

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

func (s *Session) open() (*engine.Handle, error) {
	if s == nil {
		return nil, ErrSessionNil
	}
	s.onceOpen.Do(func() {
		if s.Root == "" {
			s.openErr = ErrProjectRootRequired
			return
		}
		s.handle, s.openErr = engine.Open(s.Root) // Open uses context.Background
	})
	return s.handle, s.openErr
}

// Project opens the project and prices once (sync.Once), then returns them.
func (s *Session) Project() (*project.Project, *prices.DB, error) {
	h, err := s.open()
	if err != nil {
		return nil, nil, err
	}
	return h.Project, h.Prices, nil
}

// Ledger returns a booked ledger by directory name, opening it once per name.
func (s *Session) Ledger(name string) (*engine.Ledger, error) {
	h, err := s.open()
	if err != nil {
		return nil, err
	}

	s.ledgerMu.Lock()
	if l, ok := s.ledgers[name]; ok {
		s.ledgerMu.Unlock()
		return l, nil
	}
	s.ledgerMu.Unlock()

	l, err := h.Ledger(context.Background(), name)
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
	h, err := s.open()
	if err != nil {
		return nil, err
	}
	return h.LedgerNames(), nil
}
