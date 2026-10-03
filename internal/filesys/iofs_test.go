package filesys

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestFromIOReadsUnderRoot(t *testing.T) {
	base := fstest.MapFS{
		"contapila.cue":      &fstest.MapFile{Data: []byte("project")},
		"personal/main.bean": &fstest.MapFile{Data: []byte("bean")},
	}
	fsys := FromIO(base, ProviderRoot)

	got, err := fsys.ReadFile(ProviderRoot + "/contapila.cue")
	require.NoError(t, err)
	require.Equal(t, "project", string(got))

	st, err := fsys.Stat(ProviderRoot)
	require.NoError(t, err)
	require.True(t, st.IsDir())

	entries, err := fsys.ReadDir(ProviderRoot)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	_, err = fsys.ReadFile("/contapila.cue")
	require.Error(t, err)
}
