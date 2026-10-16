// Package ignore collects a repository's gitignore rules.
//
// Matching is github.com/lewtec/lewkit/x/fs/ignore. This package only
// chooses the directory that collector reads: the git worktree that
// contains start, or start's own directory when there is no worktree.
package ignore

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	fsignore "github.com/lewtec/lewkit/x/fs/ignore"
)

// ErrNilContext is returned when [Open] is called with a nil context.
var ErrNilContext = errors.New("nil context")

// Rules is the lewkit ignore filesystem for one tree.
// A nil Rules ignores nothing.
type Rules struct {
	// FS hides ignored names. Paths are slash-separated, relative to Root.
	FS *fsignore.FS
	// Root is the absolute directory FS is rooted at.
	Root string
}

// Open reads the ignore rules that apply to start.
// start may be a file or a directory. Inside a git worktree, Root is the
// worktree root, so a parent .gitignore applies. Without a worktree, Root
// is start's directory.
func Open(ctx context.Context, start string) (*Rules, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	dir, err := startDir(start)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// A virtual root such as the content provider path is not on disk.
			// Scanning its parent would walk the filesystem root.
			return &Rules{}, nil
		}
		return nil, err
	}
	root := worktreeRoot(dir)
	collected, err := fsignore.Collect(ctx, os.DirFS(root), fsignore.Config{
		// .git is not a ledger path. The collector already refuses to
		// descend into it while reading rules.
		SkipDirs: []string{".git"},
	})
	if err != nil {
		return nil, err
	}
	return &Rules{FS: collected, Root: root}, nil
}

// Ignored reports whether path is skipped.
// Paths outside Root, and Root itself, are not ignored.
func (r *Rules) Ignored(path string) bool {
	if r == nil || r.FS == nil || r.Root == "" {
		return false
	}
	rel, ok := relUnder(r.Root, path)
	if !ok {
		return false
	}
	return r.FS.Ignored(rel)
}

func startDir(start string) (string, error) {
	if strings.TrimSpace(start) == "" {
		return "", fs.ErrInvalid
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return filepath.Dir(abs), nil
	}
	return abs, nil
}

func worktreeRoot(dir string) string {
	for d := dir; ; {
		if isGitWorkTree(d) {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir
		}
		d = parent
	}
}

func isGitWorkTree(dir string) bool {
	st, err := os.Lstat(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	return st.IsDir() || st.Mode().IsRegular()
}

func relUnder(root, path string) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}
