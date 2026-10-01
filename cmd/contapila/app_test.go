package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestLazyProjectHandlerMissingProject(t *testing.T) {
	t.Chdir(t.TempDir())
	handler := lazyProjectHandler(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "contapila.cue")
	require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
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
