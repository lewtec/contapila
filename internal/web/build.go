package web

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/lucasew/contapila-go/internal/engine"
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
// Progress is logged with slog (Info for pages and phase summaries; Debug for
// static assets and docs). Use contapila --verbose for asset-level detail.
func Build(root, outDir string) error {
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

	start := time.Now()
	slog.Info("build start", "root", root, "out", absOut)

	ctx, err := NewSiteCtx(root)
	if err != nil {
		return err
	}
	ledgers := engine.LedgerNames(ctx.Project)
	slog.Info("build project open",
		"ledgers", len(ledgers),
		"ledger_names", ledgers,
	)

	s, err := New(ctx.Project, ctx.Prices)
	if err != nil {
		return err
	}
	// Align Server.Root with expand context (New already sets it from project).
	if s.Root == "" {
		s.Root = root
	}

	reg := DefaultRegistry(s)
	expandStart := time.Now()
	insts, err := reg.Instances(ctx)
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

	h := s.Handler()

	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return fmt.Errorf("web: mkdir %s: %w", absOut, err)
	}

	var wrotePage, wroteStatic, wroteDoc int
	var bytesTotal int64
	renderStart := time.Now()
	total := len(insts)
	for i, inst := range insts {
		n, err := writeInstance(absOut, h, inst)
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
		bytesTotal += n
		switch inst.Kind {
		case KindPage:
			wrotePage++
			slog.Info("build page",
				"path", inst.Path,
				"bytes", n,
				"index", wrotePage,
				"pages", nPage,
				"progress", fmt.Sprintf("%d/%d", i+1, total),
			)
		case KindStatic:
			wroteStatic++
			slog.Debug("build static",
				"path", inst.Path,
				"bytes", n,
				"index", wroteStatic,
				"static", nStatic,
			)
		case KindDoc:
			wroteDoc++
			slog.Debug("build doc",
				"path", inst.Path,
				"bytes", n,
				"index", wroteDoc,
				"docs", nDoc,
			)
		}
	}

	slog.Info("build complete",
		"out", absOut,
		"pages", wrotePage,
		"static", wroteStatic,
		"docs", wroteDoc,
		"bytes", bytesTotal,
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

func writeInstance(outDir string, h http.Handler, inst Instance) (int64, error) {
	rel, err := fileRel(inst)
	if err != nil {
		return 0, err
	}
	dest := filepath.Join(outDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, fmt.Errorf("web: mkdir for %s: %w", inst.Path, err)
	}

	req := httptest.NewRequest(http.MethodGet, inst.Path, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		return 0, fmt.Errorf("%w: %s status %d", ErrBuildStatus, inst.Path, rr.Code)
	}
	body := rr.Body.Bytes()
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		return 0, fmt.Errorf("web: write %s: %w", dest, err)
	}
	return int64(len(body)), nil
}
