package web

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lucasew/contapila-go/internal/engine"
	"golang.org/x/sync/errgroup"
)

// Build sentinel errors.
var (
	ErrBuildOutRequired = errors.New("web: build output directory is required")
	ErrBuildStatus      = errors.New("web: build path returned non-OK status")
)

// Build writes a static site for the project at root into outDir.
// Pages use empty time filters (all-time / as-of latest). Live-only routes
// (ledger root redirects) are omitted; report pages include check instead.
//
// jobs caps concurrent render workers. If jobs <= 0, GOMAXPROCS is used.
// One Session is shared across workers (Project Once + ledger map mutex).
//
// Progress is logged with slog (Info for pages and phase summaries; Debug for
// static assets and docs). Use contapila --verbose for asset-level detail.
func Build(root, outDir string, jobs int) error {
	if root == "" {
		return ErrProjectRootRequired
	}
	if outDir == "" {
		return ErrBuildOutRequired
	}
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return fmt.Errorf("web: build out: %w", err)
	}
	if jobs <= 0 {
		jobs = runtime.GOMAXPROCS(0)
	}
	if jobs < 1 {
		jobs = 1
	}

	start := time.Now()
	slog.Info("build start", "root", root, "out", absOut, "jobs", jobs)

	// One Session for expand + every page: OpenProject / OpenLedger warm once.
	sess := NewSession(root)
	p, pdb, err := sess.Project()
	if err != nil {
		return err
	}
	ledgers := engine.LedgerNames(p)
	slog.Info("build project open",
		"ledgers", len(ledgers),
		"ledger_names", ledgers,
	)

	s, err := New(p, pdb)
	if err != nil {
		return err
	}
	if s.Root == "" {
		s.Root = root
	}

	reg := DefaultRegistry(s)
	expandStart := time.Now()
	insts, err := reg.Instances(sess)
	if err != nil {
		return err
	}
	var nPage, nStatic, nDoc int
	for _, inst := range insts {
		switch inst.Kind {
		case KindPage:
			nPage++
		case KindStatic:
			nStatic++
		case KindDoc:
			nDoc++
		}
	}
	slog.Info("build routes expanded",
		"total", len(insts),
		"pages", nPage,
		"static", nStatic,
		"docs", nDoc,
		"duration", time.Since(expandStart).Round(time.Millisecond),
	)

	// Warm every ledger before the pool so workers only read the map.
	for _, name := range ledgers {
		if _, err := sess.Ledger(name); err != nil {
			return err
		}
	}

	h := s.Handler()

	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return fmt.Errorf("web: mkdir %s: %w", absOut, err)
	}

	var (
		wrotePage   atomic.Int64
		wroteStatic atomic.Int64
		wroteDoc    atomic.Int64
		bytesTotal  atomic.Int64
		done        atomic.Int64
	)
	renderStart := time.Now()
	total := len(insts)

	g := new(errgroup.Group)
	g.SetLimit(jobs)
	for i, inst := range insts {
		i, inst := i, inst
		g.Go(func() error {
			n, err := writeInstance(absOut, h, sess, inst)
			if err != nil {
				slog.Error("build path failed",
					"path", inst.Path,
					"kind", kindLabel(inst.Kind),
					"index", i+1,
					"total", total,
					"err", err,
				)
				return err
			}
			bytesTotal.Add(n)
			completed := done.Add(1)
			switch inst.Kind {
			case KindPage:
				idx := wrotePage.Add(1)
				slog.Info("build page",
					"path", inst.Path,
					"bytes", n,
					"index", idx,
					"pages", nPage,
					"progress", fmt.Sprintf("%d/%d", completed, total),
				)
			case KindStatic:
				idx := wroteStatic.Add(1)
				slog.Debug("build static",
					"path", inst.Path,
					"bytes", n,
					"index", idx,
					"static", nStatic,
				)
			case KindDoc:
				idx := wroteDoc.Add(1)
				slog.Debug("build doc",
					"path", inst.Path,
					"bytes", n,
					"index", idx,
					"docs", nDoc,
				)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	slog.Info("build complete",
		"out", absOut,
		"pages", wrotePage.Load(),
		"static", wroteStatic.Load(),
		"docs", wroteDoc.Load(),
		"bytes", bytesTotal.Load(),
		"jobs", jobs,
		"duration", time.Since(start).Round(time.Millisecond),
		"render", time.Since(renderStart).Round(time.Millisecond),
	)
	return nil
}

func kindLabel(k Kind) string {
	switch k {
	case KindPage:
		return "page"
	case KindStatic:
		return "static"
	case KindDoc:
		return "doc"
	default:
		return fmt.Sprintf("kind(%d)", k)
	}
}

func writeInstance(outDir string, h http.Handler, sess *Session, inst Instance) (int64, error) {
	rel, err := fileRel(inst)
	if err != nil {
		return 0, err
	}
	dest := filepath.Join(outDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, fmt.Errorf("web: mkdir for %s: %w", inst.Path, err)
	}

	body, err := fetchBuildBody(h, sess, inst.Path)
	if err != nil {
		return 0, err
	}
	if inst.Kind == KindPage {
		body = rewriteStaticPageLinks(body)
	}
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		return 0, fmt.Errorf("web: write %s: %w", dest, err)
	}
	return int64(len(body)), nil
}

// staticPageLinkRe matches href/action to root-absolute app paths (not static/docfile).
// Groups: 1=attr prefix, 2=path, 3=optional query/fragment + closing quote material handled separately.
var staticPageLinkRe = regexp.MustCompile(`((?:href|action)=")(/l/[^"?#]*)([?#][^"]*)?"`)

// rewriteStaticPageLinks turns live extensionless page URLs into *.html files
// written by fileRel, e.g. /l/acme/check → /l/acme/check.html.
// Directory URLs (trailing /) and non-/l/ paths are left alone.
func rewriteStaticPageLinks(body []byte) []byte {
	return staticPageLinkRe.ReplaceAllFunc(body, func(m []byte) []byte {
		sub := staticPageLinkRe.FindSubmatch(m)
		if sub == nil {
			return m
		}
		prefix, p, suffix := string(sub[1]), string(sub[2]), string(sub[3])
		if p == "" || strings.HasSuffix(p, "/") || strings.HasSuffix(p, ".html") {
			return m
		}
		// /l/{ledger}/account/… and /commodity/… and report pages all get .html
		return []byte(prefix + p + ".html" + suffix + `"`)
	})
}

// fetchBuildBody GETs path via h with sess on the request context so handlers
// reuse the build Session. One 3xx hop is followed (ledger root → check).
func fetchBuildBody(h http.Handler, sess *Session, path string) ([]byte, error) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if sess != nil {
		req = req.WithContext(withSession(req.Context(), sess))
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code >= 300 && rr.Code < 400 {
		loc := rr.Header().Get("Location")
		if loc == "" {
			return nil, fmt.Errorf("%w: %s status %d (no Location)", ErrBuildStatus, path, rr.Code)
		}
		req2 := httptest.NewRequest(http.MethodGet, loc, nil)
		if sess != nil {
			req2 = req2.WithContext(withSession(req2.Context(), sess))
		}
		rr2 := httptest.NewRecorder()
		h.ServeHTTP(rr2, req2)
		if rr2.Code != http.StatusOK {
			return nil, fmt.Errorf("%w: %s → %s status %d", ErrBuildStatus, path, loc, rr2.Code)
		}
		return rr2.Body.Bytes(), nil
	}
	if rr.Code != http.StatusOK {
		return nil, fmt.Errorf("%w: %s status %d", ErrBuildStatus, path, rr.Code)
	}
	return rr.Body.Bytes(), nil
}
