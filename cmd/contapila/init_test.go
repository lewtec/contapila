package main

import (
	"os"
	"strings"
	"testing"

	"github.com/lewtec/lewkit/x/cmd"
	lewpath "github.com/lewtec/lewkit/x/path"
	lewtest "github.com/lewtec/lewkit/x/test"
	"github.com/lucasew/contapila-go/internal/initfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openDestination(t *testing.T) *lewpath.Root {
	t.Helper()
	destination, err := lewpath.Open(t.TempDir())
	require.NoError(t, err)
	lewtest.CloseOnCleanup(t, destination)
	return destination
}

func TestInitEmptyDir(t *testing.T) {
	destination := openDestination(t)
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", destination.Name(), "init")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "wrote starter project to")
	_, err := lewpath.New("contapila.cue").Stat(destination)
	require.NoError(t, err)
	_, err = lewpath.New("personal", "main.beancount").Stat(destination)
	require.NoError(t, err)
	_, err = lewpath.New("company", "main.beancount").Stat(destination)
	require.NoError(t, err)
}

func TestInitIgnoresGit(t *testing.T) {
	destination := openDestination(t)
	require.NoError(t, lewpath.New(".git").Mkdir(destination, 0o755))
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", destination.Name(), "init")
	lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	ok, err := lewpath.New(".git").IsDir(destination)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestInitRefusesNonEmpty(t *testing.T) {
	destination := openDestination(t)
	require.NoError(t, lewpath.New(".git").Mkdir(destination, 0o755))
	require.NoError(t, lewpath.New(".gitignore").WriteFile(destination, []byte("*\n"), 0o644))
	require.NoError(t, lewpath.New("README.md").WriteFile(destination, []byte("hi\n"), 0o644))
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", destination.Name(), "init")
	err := app.Run(t.Context())
	require.ErrorIs(t, err, initfs.ErrNotEmpty)
	assert.Contains(t, err.Error(), ".gitignore")
	assert.Contains(t, err.Error(), "README.md")
	for line := range strings.SplitSeq(err.Error(), "\n") {
		assert.NotEqual(t, ".git", line)
	}
	ok, err := lewpath.New("contapila.cue").Exists(destination)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestInitForceAllowsExtra(t *testing.T) {
	destination := openDestination(t)
	require.NoError(t, lewpath.New("README.md").WriteFile(destination, []byte("keep me\n"), 0o644))
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", destination.Name(), "init", "--force")
	lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	_, err := lewpath.New("contapila.cue").Stat(destination)
	require.NoError(t, err)
	got, err := lewpath.New("README.md").ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, "keep me\n", string(got))
}

func TestInitForceCollision(t *testing.T) {
	destination := openDestination(t)
	require.NoError(t, lewpath.New("contapila.cue").WriteFile(destination, []byte("old\n"), 0o644))
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", destination.Name(), "init", "--force")
	require.ErrorIs(t, app.Run(t.Context()), os.ErrExist)
	got, err := lewpath.New("contapila.cue").ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, "old\n", string(got))
}

func TestInitThenStatus(t *testing.T) {
	destination := openDestination(t)
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", destination.Name(), "init")
	lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	app = cmd.ParseOK[cmd.App[root]](t, "-C", destination.Name(), "status")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "Ledgers (2):")
	assert.Contains(t, out, "personal")
	assert.Contains(t, out, "company")
	app = cmd.ParseOK[cmd.App[root]](t, "-C", destination.Name(), "check")
	out = lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "== personal ==")
	assert.Contains(t, out, "== company ==")
	assert.GreaterOrEqual(t, strings.Count(out, "OK"), 2)
}

func TestHelpListsInit(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "init", "--help")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "--force")
	assert.Contains(t, out, "starter")
}
