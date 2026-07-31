package project

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/filesys"
	"github.com/lucasew/contapila-go/internal/loader"
	"github.com/lucasew/contapila-go/internal/prices"
)

// Sentinel errors for project discovery and journal path resolution.
var (
	ErrEmptyJournalPath    = errors.New("project_journals path is empty")
	ErrJournalPathAbsolute = errors.New("project_journals path must be relative to project root")
	ErrJournalPathEscapes  = errors.New("project_journals path escapes project root")
	ErrNotAProject         = errors.New("not a contapila project")
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

// resolveProjectJournalPath resolves a project_journals path under root.
// SPEC: paths are relative to the project root. Absolute paths and ".."
// components that escape the root are rejected so a hostile contapila.cue
// cannot pull arbitrary files into prices/stream injection.
func resolveProjectJournalPath(root, rel string) (abs string, err error) {
	if rel == "" {
		return "", ErrEmptyJournalPath
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: %q", ErrJournalPathAbsolute, rel)
	}
	cleaned := filepath.Clean(rel)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: %q", ErrJournalPathEscapes, rel)
	}
	abs = filepath.Join(root, cleaned)
	// Double-check after Join (Windows volume quirks, odd cleans).
	relToRoot, err := filepath.Rel(root, abs)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: %q", ErrJournalPathEscapes, rel)
	}
	return abs, nil
}

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

	return "", fmt.Errorf("%w (searched upward for %s)", ErrNotAProject, ProjectMarker)
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
		return nil, fmt.Errorf("discover ledgers: %w", err)
	}
	discovered := make([]config.Ledger, 0, len(ledgers))
	for _, l := range ledgers {
		discovered = append(discovered, config.Ledger{Name: l.Name, Main: l.MainPath})
	}

	cuePath := filepath.Join(root, ProjectMarker)
	cueBytes, err := fsys.ReadFile(cuePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ProjectMarker, err)
	}

	// First unify without price_pairs / journal plugins so we can read project_journals.
	cfg, err := config.Load(cueBytes, cuePath, discovered, nil)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	journals, err := config.ProjectJournals(cfg.Value)
	if err != nil {
		return nil, fmt.Errorf("read project_journals: %w", err)
	}
	var (
		pricePairs     []config.PricePair
		pricesPath     string
		pricesMissing  bool
		pricesEmpty    bool
		streamJournals []StreamJournal
	)

	for _, j := range journals {
		abs, err := resolveProjectJournalPath(root, j.Path)
		if err != nil {
			return nil, err
		}
		info, err := fsys.Stat(abs)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if j.Missing == "warn" {
				slog.Warn("project journal missing", "path", abs, "role", j.Role)
			}
			if j.Role == "prices" && pricesPath == "" {
				pricesPath = abs
				pricesMissing = true
			}
			continue
		case err != nil:
			return nil, fmt.Errorf("stat %s: %w", abs, err)
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

	// Journal plugin "name" directives → plugins.<name>.enabled: true (config plane).
	journalPlugins := collectJournalPluginNames(fsys, ledgers, streamJournals)

	// Re-unify with price_pairs and/or journal plugins when present.
	if len(pricePairs) > 0 || len(journalPlugins) > 0 {
		cfg2, err := config.LoadWithPlugins(cueBytes, cuePath, discovered, pricePairs, journalPlugins)
		if err != nil {
			return nil, fmt.Errorf("load config with injects: %w", err)
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

// collectJournalPluginNames loads each ledger main (+ stream journals) and
// returns unique known plugin directive names (e.g. "web/accounts").
// Unknown names are skipped with a slog warning (file/line when available).
func collectJournalPluginNames(fsys filesys.FS, ledgers []Ledger, streams []StreamJournal) []string {
	if fsys == nil {
		fsys = filesys.OS{}
	}
	seen := map[string]bool{}
	var out []string
	add := func(path string) {
		if path == "" {
			return
		}
		dirs, _, err := loader.LoadFileFS(fsys, path)
		if err != nil {
			slog.Warn("scan plugins: load failed", "path", path, "err", err)
			return
		}
		for _, d := range dirs {
			p, ok := d.(ast.Plugin)
			if !ok || p.Name == "" {
				continue
			}
			if !config.IsKnownPlugin(p.Name) {
				// Once per name; first sighting keeps location for the warn.
				if !seen[p.Name] {
					seen[p.Name] = true
					slog.Warn("unknown plugin directive",
						"plugin", p.Name,
						"file", p.File,
						"line", p.Line,
						"known", config.KnownPluginIDs(),
					)
				}
				continue
			}
			if seen[p.Name] {
				continue
			}
			seen[p.Name] = true
			out = append(out, p.Name)
		}
	}
	for _, l := range ledgers {
		add(l.MainPath)
	}
	for _, s := range streams {
		add(s.Path)
	}
	return out
}
