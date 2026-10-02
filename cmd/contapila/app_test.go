package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lewtec/lewkit/x/ui/gui"
	"github.com/lucasew/contapila-go/pkg/project"
	"github.com/stretchr/testify/require"
)

func TestWantWindowUnstamped(t *testing.T) {
	require.False(t, wantWindow(nil))
	for _, args := range [][]string{
		{"--help"},
		{"version"},
		{"desktop"},
		{"-C", t.TempDir()},
		{"status"},
	} {
		require.False(t, wantWindow(args), "args %q", args)
	}
}

func TestStampedVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		version string
		want    bool
	}{
		{version: "", want: false},
		{version: "dev", want: false},
		{version: "dev-abcdef12", want: false},
		{version: "0.4.1", want: true},
		{version: "0.4.1-abcdef12", want: true},
		{version: "v1.2.3", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, stampedVersion(tt.version))
		})
	}
}

func TestWelcomeMissingProject(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	handler := projectHandler(t.Context())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	body := rec.Body.String()
	require.Contains(t, body, "Choose a folder")
	require.Contains(t, body, "Open a folder")
	require.Contains(t, body, "contapila.cue")
}

func withWelcome(t *testing.T, fn welcomeOpener) {
	t.Helper()
	previous := openWelcome
	openWelcome = fn
	t.Cleanup(func() { openWelcome = previous })
}

func TestInteractiveProjectSkipsFolderWindow(t *testing.T) {
	example := exampleProject(t)
	isolateWelcome(t)
	t.Chdir(example)
	withWelcome(t, func(context.Context, string, []gui.Directory) (string, error) {
		require.FailNow(t, "folder window opened for a project")
		return "", nil
	})
	handler, err := interactiveHandler(t.Context())
	require.NoError(t, err)
	require.Contains(t, serveRoot(t, handler), "Ledgers")
}

func TestInteractiveWelcomeOpensExample(t *testing.T) {
	example := exampleProject(t)
	isolateWelcome(t)
	withWelcome(t, func(context.Context, string, []gui.Directory) (string, error) {
		return example, nil
	})
	handler, err := interactiveHandler(t.Context())
	require.NoError(t, err)
	require.Contains(t, serveRoot(t, handler), "Ledgers")

	recentPath := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "lewkit", "recent-dirs")
	text, err := os.ReadFile(recentPath)
	require.NoError(t, err)
	require.Equal(t, example+"\n", string(text))
}

func TestInteractiveWelcomeCancel(t *testing.T) {
	isolateWelcome(t)
	withWelcome(t, func(context.Context, string, []gui.Directory) (string, error) {
		return "", nil
	})
	handler, err := interactiveHandler(t.Context())
	require.NoError(t, err)
	require.Nil(t, handler)
}

func TestInteractiveWelcomeRejectsThenAccepts(t *testing.T) {
	example := exampleProject(t)
	isolateWelcome(t)
	var titles []string
	withWelcome(t, func(_ context.Context, title string, _ []gui.Directory) (string, error) {
		titles = append(titles, title)
		if len(titles) == 1 {
			return t.TempDir(), nil
		}
		return example, nil
	})
	handler, err := interactiveHandler(t.Context())
	require.NoError(t, err)
	require.Contains(t, serveRoot(t, handler), "Ledgers")
	require.Equal(t, []string{"Contapila", "not a contapila project (searched upward for contapila.cue)"}, titles)
}

func TestInteractiveWelcomeError(t *testing.T) {
	isolateWelcome(t)
	withWelcome(t, func(context.Context, string, []gui.Directory) (string, error) {
		return "", errPortalBroke
	})
	handler, err := interactiveHandler(t.Context())
	require.ErrorIs(t, err, errPortalBroke)
	require.Nil(t, handler)
}

func TestInteractiveBrokenProjectSkipsFolderWindow(t *testing.T) {
	isolateWelcome(t)
	require.NoError(t, os.WriteFile("contapila.cue", []byte("not cue :::"), 0o644))
	called := false
	withWelcome(t, func(context.Context, string, []gui.Directory) (string, error) {
		called = true
		return "", nil
	})
	handler, err := interactiveHandler(t.Context())
	require.Error(t, err)
	require.NotErrorIs(t, err, project.ErrNotAProject)
	require.Nil(t, handler)
	require.False(t, called)
}

func serveRoot(t *testing.T, handler http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	return rec.Body.String()
}

func TestHeadlessHost(t *testing.T) {
	t.Setenv("LEWKIT_NO_UI", "")
	t.Setenv("ELETROCROMO_NO_UI", "")
	require.False(t, headlessHost())
	t.Setenv("LEWKIT_NO_UI", "1")
	require.True(t, headlessHost())
	t.Setenv("LEWKIT_NO_UI", "")
	t.Setenv("ELETROCROMO_NO_UI", "yes")
	require.True(t, headlessHost())
}
