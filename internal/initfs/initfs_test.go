package initfs

import (
	"io/fs"
	"testing"
	"testing/fstest"

	lewpath "github.com/lewtec/lewkit/x/path"
	lewtest "github.com/lewtec/lewkit/x/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOccupantsIgnoresGitOnly(t *testing.T) {
	t.Parallel()
	root, err := lewpath.Open(t.TempDir())
	require.NoError(t, err)
	lewtest.CloseOnCleanup(t, root)
	require.NoError(t, lewpath.New(".git").Mkdir(root, 0o755))
	got, err := Occupants(root)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestOccupantsListsNonGit(t *testing.T) {
	t.Parallel()
	root, err := lewpath.Open(t.TempDir())
	require.NoError(t, err)
	lewtest.CloseOnCleanup(t, root)
	require.NoError(t, lewpath.New(".git").Mkdir(root, 0o755))
	for _, name := range []string{".gitignore", "README.md", "foo"} {
		require.NoError(t, lewpath.New(name).WriteFile(root, []byte("x\n"), 0o644))
	}
	got, err := Occupants(root)
	require.NoError(t, err)
	assert.Equal(t, []string{".gitignore", "README.md", "foo"}, got)
}

func TestCopyWrites(t *testing.T) {
	t.Parallel()
	src := fstest.MapFS{
		"contapila.cue":           &fstest.MapFile{Data: []byte("cue\n")},
		"personal/main.beancount": &fstest.MapFile{Data: []byte("main\n")},
		".helix/languages.toml":   &fstest.MapFile{Data: []byte("helix\n")},
	}
	root, err := lewpath.Open(t.TempDir())
	require.NoError(t, err)
	lewtest.CloseOnCleanup(t, root)
	require.NoError(t, Copy(t.Context(), root, src))
	checks := map[string]string{
		"contapila.cue":           "cue\n",
		"personal/main.beancount": "main\n",
		".helix/languages.toml":   "helix\n",
	}
	for rel, want := range checks {
		got, err := lewpath.New(rel).ReadFile(root)
		require.NoError(t, err, rel)
		assert.Equal(t, want, string(got), rel)
	}
}

func TestCopyExist(t *testing.T) {
	t.Parallel()
	root, err := lewpath.Open(t.TempDir())
	require.NoError(t, err)
	lewtest.CloseOnCleanup(t, root)
	require.NoError(t, lewpath.New("contapila.cue").WriteFile(root, []byte("old\n"), 0o644))
	err = Copy(t.Context(), root, fstest.MapFS{
		"contapila.cue": &fstest.MapFile{Data: []byte("cue\n")},
	})
	require.ErrorIs(t, err, fs.ErrExist)
	got, err := lewpath.New("contapila.cue").ReadFile(root)
	require.NoError(t, err)
	assert.Equal(t, "old\n", string(got))
}

func TestExampleHasProjectMarker(t *testing.T) {
	t.Parallel()
	src, err := Example()
	require.NoError(t, err)
	b, err := lewpath.New("contapila.cue").ReadFile(src)
	require.NoError(t, err)
	assert.Contains(t, string(b), "commodities")
	assert.Contains(t, string(b), "company-aporte")
	assert.Contains(t, string(b), "company-profit-distribution")
}
