package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func open(t *testing.T, start string) *Rules {
	t.Helper()
	rules, err := Open(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func TestOpenSkipsGitignorePatterns(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".gitignore", "*.draft\nsecret/\n")
	writeFile(t, root, "keep.txt", "k")
	writeFile(t, root, "note.draft", "d")
	writeFile(t, root, "secret/hidden.txt", "h")
	writeFile(t, root, "docs/.gitignore", "*.pdf\n!keep.pdf\n")
	writeFile(t, root, "docs/a.pdf", "p")
	writeFile(t, root, "docs/keep.pdf", "p")
	writeFile(t, root, "docs/b.txt", "t")

	rules := open(t, root)
	if rules.Ignored(filepath.Join(root, "keep.txt")) {
		t.Fatal("keep.txt should not be ignored")
	}
	if !rules.Ignored(filepath.Join(root, "note.draft")) {
		t.Fatal("note.draft should be ignored")
	}
	if !rules.Ignored(filepath.Join(root, "secret")) {
		t.Fatal("secret/ should be ignored")
	}
	if !rules.Ignored(filepath.Join(root, "secret", "hidden.txt")) {
		t.Fatal("file under ignored directory should be ignored")
	}
	if !rules.Ignored(filepath.Join(root, "docs", "a.pdf")) {
		t.Fatal("nested gitignore should ignore docs/a.pdf")
	}
	if rules.Ignored(filepath.Join(root, "docs", "keep.pdf")) {
		t.Fatal("nested negation should keep docs/keep.pdf")
	}
	if rules.Ignored(filepath.Join(root, "docs", "b.txt")) {
		t.Fatal("docs/b.txt should not be ignored")
	}
}

func TestOpenUsesParentWorktreeGitignore(t *testing.T) {
	root := t.TempDir()
	// .git directory marks the worktree. A .gitignore above the project applies.
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, ".gitignore", "skip-me/\n")
	proj := filepath.Join(root, "proj")
	writeFile(t, proj, "skip-me/file.txt", "x")
	writeFile(t, proj, "keep/file.txt", "y")

	rules := open(t, proj)
	if !rules.Ignored(filepath.Join(proj, "skip-me")) {
		t.Fatal("parent worktree pattern should ignore skip-me/")
	}
	if rules.Ignored(filepath.Join(proj, "keep", "file.txt")) {
		t.Fatal("keep/file.txt should not be ignored")
	}
}

func TestOpenExcludeOverriddenByGitignore(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, ".git/info/exclude", "*.tmp\n")
	writeFile(t, root, ".gitignore", "!keep.tmp\n")
	writeFile(t, root, "drop.tmp", "d")
	writeFile(t, root, "keep.tmp", "k")

	rules := open(t, root)
	if !rules.Ignored(filepath.Join(root, "drop.tmp")) {
		t.Fatal("exclude should ignore drop.tmp")
	}
	if rules.Ignored(filepath.Join(root, "keep.tmp")) {
		t.Fatal(".gitignore negation should override exclude for keep.tmp")
	}
}

func TestIgnoredOutsideRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".gitignore", "*\n")
	other := t.TempDir()
	writeFile(t, other, "file.txt", "x")

	rules := open(t, root)
	if rules.Ignored(filepath.Join(other, "file.txt")) {
		t.Fatal("path outside the root should not be ignored")
	}
	if rules.Ignored(root) {
		t.Fatal("the root directory itself should not be ignored")
	}
}

func TestGitDirAlwaysIgnored(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	rules := open(t, root)
	if !rules.Ignored(filepath.Join(root, ".git")) {
		t.Fatal(".git directory should be ignored")
	}
}

func TestNoRulesIgnoresNothing(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.txt", "a")
	rules := open(t, root)
	if rules.Ignored(filepath.Join(root, "a.txt")) {
		t.Fatal("without rules, a.txt should not be ignored")
	}
	var none *Rules
	if none.Ignored(filepath.Join(root, "a.txt")) {
		t.Fatal("nil Rules should ignore nothing")
	}
}

func TestOpenMissingPathIgnoresNothing(t *testing.T) {
	root := t.TempDir()
	rules, err := Open(t.Context(), filepath.Join(root, "missing", "nope.beancount"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "secret.txt", "s")
	if rules.Ignored(filepath.Join(root, "secret.txt")) {
		t.Fatal("a missing start path should not invent ignore rules")
	}
}

func TestParentGitignoreWithoutWorktreeDoesNotApply(t *testing.T) {
	parent := t.TempDir()
	writeFile(t, parent, ".gitignore", "secret.txt\n")
	child := filepath.Join(parent, "child")
	writeFile(t, child, "secret.txt", "s")

	rules := open(t, child)
	if rules.Ignored(filepath.Join(child, "secret.txt")) {
		t.Fatal("a parent .gitignore should not apply without a .git worktree")
	}
}
