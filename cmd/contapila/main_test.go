package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/lewkit/x/release"
	lewtest "github.com/lewtec/lewkit/x/test"
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exampleDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "example"))
	require.NoError(t, err)
	st, err := os.Stat(dir)
	require.NoError(t, err)
	require.True(t, st.IsDir())
	return dir
}

func TestTreePadMark(t *testing.T) {
	pad, mark := treePadMark(2, true)
	if pad != "    " || mark != "Σ " {
		t.Fatalf("rollup: pad=%q mark=%q", pad, mark)
	}
	pad, mark = treePadMark(0, false)
	if pad != "" || mark != "  " {
		t.Fatalf("leaf: pad=%q mark=%q", pad, mark)
	}
}

func TestStatusExample(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", exampleDir(t), "status")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "Project root:")
	assert.Contains(t, out, "Ledgers (4):")
	assert.Contains(t, out, "personal")
	assert.Contains(t, out, "acme")
	assert.Contains(t, out, "CUE:               Unified OK")
}

func TestCheckExample(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", exampleDir(t), "check")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.GreaterOrEqual(t, strings.Count(out, "OK"), 4)
	for _, name := range []string{"acme", "ong", "personal", "smuggle"} {
		assert.Contains(t, out, "== "+name+" ==")
	}
}

func TestCheckExampleSingleLedger(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", exampleDir(t), "check", "personal")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "== personal ==")
	assert.Contains(t, out, "OK")
	assert.NotContains(t, out, "== acme ==")
}

func TestParseCommodities(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "testdata", "example", "commodities.beancount"))
	require.NoError(t, err)
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "parse", path)
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "ast.Commodity")
}

func TestHelpListsCommands(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "--help")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	for _, want := range []string{"status", "check", "init", "dump", "web", "desktop", "version", "--directory"} {
		assert.Contains(t, out, want)
	}
}

func TestVersionFlag(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "--version")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Equal(t, release.Version(), strings.TrimSpace(out))
}

func TestVersionCommand(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "version")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Equal(t, release.Version(), strings.TrimSpace(out))
}

func TestUnknownLedger(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "-C", exampleDir(t), "check", "nope")
	require.ErrorIs(t, app.Run(t.Context()), engine.ErrUnknownLedger)
}

func TestDirectoryEnv(t *testing.T) {
	t.Setenv("CONTAPILA_DIRECTORY", exampleDir(t))
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "status")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "Ledgers (4):")
}

func TestDirectoryAfterCommand(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "status", "-C", exampleDir(t))
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "Ledgers (4):")
}

func TestDirectoryAfterLedger(t *testing.T) {
	dir := exampleDir(t)
	t.Chdir(t.TempDir())
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "check", "personal", "-C", dir)
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "== personal ==")
}

func TestDirectoryFlagMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-project-root")
	err := cmd.ParseErr[cmd.App[root]](t, "-C", missing, "status")
	require.ErrorIs(t, err, fs.ErrNotExist)
	assert.True(t, strings.Contains(err.Error(), "-C") || strings.Contains(err.Error(), "--directory"), err.Error())
}
