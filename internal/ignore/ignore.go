// Package ignore reports paths skipped by a repository's gitignore rules.
//
// lewkit has no gitignore matcher. Matching uses github.com/git-pkgs/gitignore
// (the library other lewtec tools already call). Rules come from the
// worktree .gitignore files and .git/info/exclude. The user's global
// excludes file is not consulted.
package ignore

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/git-pkgs/gitignore"
)

// Rules is a gitignore matcher rooted at a worktree (or at start, when
// there is no .git). A nil Rules ignores nothing.
type Rules struct {
	root string
	m    *gitignore.Matcher
}

// Load reads ignore rules that apply to start. start may be a file or a
// directory. The root is the nearest .git worktree walking upward, or
// start's directory when there is none, so a .gitignore next to the
// files still applies.
func Load(start string) *Rules {
	dir := filepath.Clean(start)
	if dir == "" || dir == "." {
		abs, err := filepath.Abs(start)
		if err != nil {
			return &Rules{}
		}
		dir = abs
	}
	if !filepath.IsAbs(dir) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return &Rules{}
		}
		dir = abs
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}

	root := findGitRoot(dir)
	if root == "" {
		root = dir
	}

	m := gitignore.New("")
	m.AddFromFile(filepath.Join(root, ".git", "info", "exclude"), "")
	m.AddFromFile(filepath.Join(root, ".gitignore"), "")
	loadAncestorGitignores(m, root, dir)
	walkNestedGitignores(m, root, dir)
	return &Rules{root: root, m: m}
}

// Ignored reports whether path is skipped by r. Directories named .git
// are always ignored. Paths outside r's root are not.
func (r *Rules) Ignored(path string, isDir bool) bool {
	if r == nil || r.m == nil || r.root == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	abs = filepath.Clean(abs)
	if isDir && filepath.Base(abs) == ".git" {
		return true
	}
	rel, err := filepath.Rel(r.root, abs)
	if err != nil || rel == "." || rel == "" {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return r.m.MatchPath(filepath.ToSlash(rel), isDir)
}

func findGitRoot(start string) string {
	dir := filepath.Clean(start)
	for {
		if isGitWorkTree(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func isGitWorkTree(root string) bool {
	st, err := os.Lstat(filepath.Join(root, ".git"))
	if err != nil {
		return false
	}
	return st.IsDir() || st.Mode().IsRegular()
}

// loadAncestorGitignores adds .gitignore files on the path from root
// (exclusive) down to start (inclusive). root's own file is loaded by Load.
func loadAncestorGitignores(m *gitignore.Matcher, root, start string) {
	if start == root {
		return
	}
	rel, err := filepath.Rel(root, start)
	if err != nil {
		return
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return
	}
	var b strings.Builder
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('/')
		}
		b.WriteString(part)
		dirRel := b.String()
		m.AddFromFile(filepath.Join(root, filepath.FromSlash(dirRel), ".gitignore"), dirRel)
	}
}

// walkNestedGitignores loads .gitignore files under start. A directory's
// rules are added before its children are considered, and an ignored
// directory is not entered.
func walkNestedGitignores(m *gitignore.Matcher, root, start string) {
	_ = filepath.WalkDir(start, func(path string, d os.DirEntry, err error) error {
		if err != nil || path == start || !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == ".." || strings.HasPrefix(rel, "../") {
			return filepath.SkipDir
		}
		if m.MatchPath(rel, true) {
			return filepath.SkipDir
		}
		m.AddFromFile(filepath.Join(path, ".gitignore"), rel)
		return nil
	})
}
