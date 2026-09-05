package project

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/filesys"
	"github.com/lucasew/contapila-go/internal/prices"
)

type Ledger struct {
	Name     string
	MainPath string
}

// StreamJournal is a project-root beancount file auto-injected into every ledger stream.
type StreamJournal struct {
	Path    string // absolute
	RelPath string // as declared in project_journals
}

type Project struct {
	Root    string
	Config  *config.Config
	Ledgers []Ledger
	// PricesPath is the first project_journals entry with role "prices" ("" if none).
	PricesPath    string
	PricesMissing bool
	PricesEmpty   bool
	// StreamJournals are role "stream" files to inject into each ledger (absolute paths).
	StreamJournals []StreamJournal
}

const ProjectMarker = "contapila.cue"
const LedgerEntrypoint = "main.beancount"

func findRoot(fsys filesys.FS, startDir string) (string, error) {
	curr, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	for {
		markerPath := filepath.Join(curr, ProjectMarker)
		if _, err := fsys.Stat(markerPath); err == nil {
			return curr, nil
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}

	return "", fmt.Errorf("not a contapila project (searched upward for %s)", ProjectMarker)
}

func discoverLedgers(fsys filesys.FS, root string) ([]Ledger, error) {
	entries, err := fsys.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var ledgers []Ledger
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		ledgerPath := filepath.Join(root, entry.Name(), LedgerEntrypoint)
		if _, err := fsys.Stat(ledgerPath); err == nil {
			ledgers = append(ledgers, Ledger{
				Name:     entry.Name(),
				MainPath: ledgerPath,
			})
		}
	}

	return ledgers, nil
}

// OpenProject opens from disk (CLI default).
func OpenProject(cwd string) (*Project, error) {
	return OpenProjectFS(filesys.OS{}, cwd)
}

// OpenProjectFS opens a project using fsys for file reads (LSP overlays).
func OpenProjectFS(fsys filesys.FS, cwd string) (*Project, error) {
	if fsys == nil {
		fsys = filesys.OS{}
	}
	root, err := findRoot(fsys, cwd)
	if err != nil {
		return nil, err
	}

	// Discover ledgers first so CUE can type #LedgerName from the filesystem.
	ledgers, err := discoverLedgers(fsys, root)
	if err != nil {
		return nil, fmt.Errorf("failed to discover ledgers: %w", err)
	}
	discovered := make([]config.Ledger, 0, len(ledgers))
	for _, l := range ledgers {
		discovered = append(discovered, config.Ledger{Name: l.Name, Main: l.MainPath})
	}

	cuePath := filepath.Join(root, ProjectMarker)
	cueBytes, err := fsys.ReadFile(cuePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", ProjectMarker, err)
	}

	// First unify without price_pairs so we can read project_journals defaults.
	cfg, err := config.Load(cueBytes, cuePath, discovered, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	journals, err := config.ProjectJournals(cfg.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to read project_journals: %w", err)
	}
	var (
		pricePairs     []config.PricePair
		pricesPath     string
		pricesMissing  bool
		pricesEmpty    bool
		streamJournals []StreamJournal
	)

	for _, j := range journals {
		abs := filepath.Join(root, j.Path)
		info, err := fsys.Stat(abs)
		switch {
		case os.IsNotExist(err):
			if j.Missing == "warn" {
				slog.Warn("project journal missing", "path", abs, "role", j.Role)
			}
			if j.Role == "prices" && pricesPath == "" {
				pricesPath = abs
				pricesMissing = true
			}
			continue
		case err != nil:
			return nil, fmt.Errorf("failed to stat %s: %w", abs, err)
		}

		if info.Size() == 0 {
			if j.Missing == "warn" {
				slog.Warn("project journal empty", "path", abs, "role", j.Role)
			}
			if j.Role == "prices" && pricesPath == "" {
				pricesPath = abs
				pricesEmpty = true
			}
			continue
		}

		switch j.Role {
		case "prices":
			if pricesPath == "" {
				pricesPath = abs
			}
			// Pair inventory for CUE (not full series).
			if pdb, _, err := prices.LoadFileFS(fsys, abs); err != nil {
				slog.Warn("failed loading prices for CUE pair inject", "path", abs, "err", err)
			} else {
				for _, p := range pdb.Pairs() {
					pricePairs = append(pricePairs, config.PricePair{Base: p.Base, Quote: p.Quote})
				}
			}
		case "stream":
			streamJournals = append(streamJournals, StreamJournal{Path: abs, RelPath: j.Path})
		}
	}

	// Re-unify with price_pairs when we discovered any (closed inventory for CUE overlays).
	if len(pricePairs) > 0 {
		cfg2, err := config.Load(cueBytes, cuePath, discovered, pricePairs)
		if err != nil {
			return nil, fmt.Errorf("failed to load config with price pairs: %w", err)
		}
		cfg = cfg2
	}

	return &Project{
		Root:           root,
		Config:         cfg,
		Ledgers:        ledgers,
		PricesPath:     pricesPath,
		PricesMissing:  pricesMissing,
		PricesEmpty:    pricesEmpty,
		StreamJournals: streamJournals,
	}, nil
}
