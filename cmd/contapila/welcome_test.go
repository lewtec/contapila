package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/lewkit/x/driver"
	"github.com/lewtec/lewkit/x/driver/filedialog"
	"github.com/stretchr/testify/require"
)

var errPortalBroke = errors.New("portal broke")

func isolateWelcome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
}

func exampleProject(t *testing.T) string {
	t.Helper()
	example, err := filepath.Abs("../../testdata/example")
	require.NoError(t, err)
	info, err := os.Stat(filepath.Join(example, "contapila.cue"))
	require.NoError(t, err)
	require.False(t, info.IsDir())
	return example
}

func postForm(handler http.Handler, target string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func withChooser(t *testing.T, choose func(context.Context) ([]string, error)) {
	t.Helper()
	previous := chooseFolder
	chooseFolder = choose
	t.Cleanup(func() { chooseFolder = previous })
}

func TestWelcomeBrowseListsChild(t *testing.T) {
	isolateWelcome(t)
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "folder-zeta"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(root, "folder-alpha"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(root, ".hidden"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(root, "folder-alpha"), filepath.Join(root, "linked")))

	handler := projectHandler(t.Context())
	req := httptest.NewRequest(http.MethodGet, browseHref(root, false), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "Open this folder")
	alpha := strings.Index(body, "folder-alpha")
	zeta := strings.Index(body, "folder-zeta")
	linked := strings.Index(body, "linked")
	require.NotEqual(t, -1, alpha)
	require.Less(t, alpha, zeta)
	require.NotEqual(t, -1, linked)
	require.NotContains(t, body, ".hidden")
	require.NotContains(t, body, "notes.txt")

	child := filepath.Join(root, "folder-alpha")
	childReq := httptest.NewRequest(http.MethodGet, browseHref(child, false), nil)
	childRec := httptest.NewRecorder()
	handler.ServeHTTP(childRec, childReq)
	require.Equal(t, http.StatusOK, childRec.Code)
	require.Contains(t, childRec.Body.String(), "Up")
	require.Contains(t, childRec.Body.String(), url.QueryEscape(root))
}

func TestWelcomeOpenEmptyDir(t *testing.T) {
	isolateWelcome(t)
	empty := t.TempDir()
	handler := projectHandler(t.Context())
	rec := postForm(handler, "/welcome/open", url.Values{"path": {empty}})
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "not a contapila project")
	require.Contains(t, body, "contapila.cue")
	require.Contains(t, body, "Choose a folder")

	again := httptest.NewRequest(http.MethodGet, "/", nil)
	againRec := httptest.NewRecorder()
	handler.ServeHTTP(againRec, again)
	require.Contains(t, againRec.Body.String(), "Choose a folder")
	require.NotContains(t, againRec.Body.String(), "Ledgers")
}

func TestWelcomeOpenExample(t *testing.T) {
	example := exampleProject(t)
	isolateWelcome(t)
	handler := projectHandler(t.Context())

	rec := postForm(handler, "/welcome/open", url.Values{"path": {example}})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "/", loc.Path)

	next := httptest.NewRequest(http.MethodGet, "/", nil)
	nextRec := httptest.NewRecorder()
	handler.ServeHTTP(nextRec, next)
	require.Equal(t, http.StatusOK, nextRec.Code)
	require.Contains(t, nextRec.Body.String(), "Ledgers")

	config := os.Getenv("XDG_CONFIG_HOME")
	recentPath := filepath.Join(config, "lewkit", "recent-dirs")
	info, err := os.Stat(recentPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	text, err := os.ReadFile(recentPath)
	require.NoError(t, err)
	require.Equal(t, example+"\n", string(text))
	dirInfo, err := os.Stat(filepath.Dir(recentPath))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
}

func TestWelcomeOpenRejectsRelativeAndFile(t *testing.T) {
	example := exampleProject(t)
	isolateWelcome(t)
	t.Chdir(filepath.Dir(example))
	handler := projectHandler(t.Context())

	relative := postForm(handler, "/welcome/open", url.Values{"path": {filepath.Base(example)}})
	require.Equal(t, http.StatusOK, relative.Code)
	require.Contains(t, relative.Body.String(), "not a folder")
	require.NotContains(t, relative.Body.String(), "Ledgers")

	file := filepath.Join(t.TempDir(), "notes.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	rejected := postForm(handler, "/welcome/open", url.Values{"path": {file}})
	require.Equal(t, http.StatusOK, rejected.Code)
	require.Contains(t, rejected.Body.String(), "not a folder")
}

func TestWelcomeRecentLink(t *testing.T) {
	isolateWelcome(t)
	dir := t.TempDir()
	config := os.Getenv("XDG_CONFIG_HOME")
	require.NoError(t, os.MkdirAll(filepath.Join(config, "lewkit"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(config, "lewkit", "recent-dirs"), []byte(dir+"\n"), 0o600))

	handler := projectHandler(t.Context())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Contains(t, rec.Body.String(), "Recent")
	require.Contains(t, rec.Body.String(), dir)

	opened := httptest.NewRequest(http.MethodGet, openHref(dir), nil)
	openedRec := httptest.NewRecorder()
	handler.ServeHTTP(openedRec, opened)
	require.Equal(t, http.StatusOK, openedRec.Code)
	require.Contains(t, openedRec.Body.String(), "contapila.cue")
	require.Contains(t, openedRec.Body.String(), "Choose a folder")
}

func TestWelcomeBrowseDialog(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		isolateWelcome(t)
		withChooser(t, func(context.Context) ([]string, error) {
			return nil, filedialog.ErrCanceled
		})
		handler := projectHandler(t.Context())
		rec := postForm(handler, "/welcome/browse", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Empty(t, rec.Header().Get("Location"))
		require.Contains(t, rec.Body.String(), "Choose a folder")
	})

	t.Run("unavailable", func(t *testing.T) {
		isolateWelcome(t)
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, "inside"), 0o755))
		t.Chdir(root)
		withChooser(t, func(context.Context) ([]string, error) {
			return nil, driver.ErrUnavailable
		})
		handler := projectHandler(t.Context())
		rec := postForm(handler, "/welcome/browse", nil)
		require.Equal(t, http.StatusSeeOther, rec.Code)
		loc, err := url.Parse(rec.Header().Get("Location"))
		require.NoError(t, err)
		require.Equal(t, "/welcome/browse", loc.Path)
		require.Equal(t, "1", loc.Query().Get("note"))
		require.Equal(t, root, loc.Query().Get("at"))

		next := httptest.NewRequest(http.MethodGet, loc.RequestURI(), nil)
		nextRec := httptest.NewRecorder()
		handler.ServeHTTP(nextRec, next)
		require.Equal(t, http.StatusOK, nextRec.Code)
		body := nextRec.Body.String()
		require.Contains(t, body, "No folder dialog on this system.")
		require.Contains(t, body, "inside")
		require.Contains(t, body, "Open this folder")
	})

	t.Run("error", func(t *testing.T) {
		isolateWelcome(t)
		withChooser(t, func(context.Context) ([]string, error) {
			return nil, errPortalBroke
		})
		handler := projectHandler(t.Context())
		rec := postForm(handler, "/welcome/browse", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "portal broke")
		require.Contains(t, rec.Body.String(), "Choose a folder")
	})

	t.Run("picked", func(t *testing.T) {
		isolateWelcome(t)
		empty := t.TempDir()
		withChooser(t, func(context.Context) ([]string, error) {
			return []string{empty}, nil
		})
		handler := projectHandler(t.Context())
		rec := postForm(handler, "/welcome/browse", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "contapila.cue")
		require.Contains(t, rec.Body.String(), "Choose a folder")
	})
}
