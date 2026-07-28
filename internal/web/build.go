package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
)

// Build sentinel errors.
var (
	ErrBuildOutRequired = errors.New("web: build output directory is required")
	ErrBuildStatus      = errors.New("web: build path returned non-OK status")
)

// Build writes a static site for the project at root into outDir.
// Pages use empty time filters (all-time / as-of latest). Live-only routes
// (ledger root redirects) are omitted; report pages include check instead.
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

	ctx, err := NewSiteCtx(root)
	if err != nil {
		return err
	}
	s, err := New(ctx.Project, ctx.Prices)
	if err != nil {
		return err
	}
	// Align Server.Root with expand context (New already sets it from project).
	if s.Root == "" {
		s.Root = root
	}

	reg := DefaultRegistry(s)
	insts, err := reg.Instances(ctx)
	if err != nil {
		return err
	}
	h := s.Handler()

	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return fmt.Errorf("web: mkdir %s: %w", absOut, err)
	}

	for _, inst := range insts {
		if err := writeInstance(absOut, h, inst); err != nil {
			return err
		}
	}
	return nil
}

func writeInstance(outDir string, h http.Handler, inst Instance) error {
	rel, err := fileRel(inst)
	if err != nil {
		return err
	}
	dest := filepath.Join(outDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("web: mkdir for %s: %w", inst.Path, err)
	}

	req := httptest.NewRequest(http.MethodGet, inst.Path, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		return fmt.Errorf("%w: %s status %d", ErrBuildStatus, inst.Path, rr.Code)
	}
	if err := os.WriteFile(dest, rr.Body.Bytes(), 0o644); err != nil {
		return fmt.Errorf("web: write %s: %w", dest, err)
	}
	return nil
}
