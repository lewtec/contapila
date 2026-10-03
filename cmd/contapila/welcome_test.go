package main

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lewtec/lewkit/x/driver"
	"github.com/lewtec/lewkit/x/driver/filedialog"
	"github.com/lewtec/lewkit/x/release"
	"github.com/stretchr/testify/require"
)

var errPortalBroke = errors.New("portal broke")

func isolateWelcome(t *testing.T) {
	t.Helper()
	// UserConfigDir is XDG_CONFIG_HOME on Linux and ~/Library/Application Support on macOS.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Chdir(t.TempDir())
}

func recentDirsPath(t *testing.T) string {
	t.Helper()
	dir, err := os.UserConfigDir()
	require.NoError(t, err)
	return filepath.Join(dir, release.Name(), "recent-dirs")
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

// settleWelcome polls until a folder dialog or project load finishes.
// The page reloads itself; tests do not wait for the refresh header.
func settleWelcome(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last *httptest.ResponseRecorder
	for {
		last = httptest.NewRecorder()
		handler.ServeHTTP(last, httptest.NewRequest(http.MethodGet, "/", nil))
		if last.Code == http.StatusSeeOther || !strings.Contains(last.Body.String(), "Opening…") {
			return last
		}
		if time.Now().After(deadline) {
			t.Fatalf("welcome still opening\n%s", last.Body.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
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
	require.Contains(t, rec.Body.String(), "Opening…")
	settled := settleWelcome(t, handler)
	require.Equal(t, http.StatusOK, settled.Code)
	body := settled.Body.String()
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
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "Opening…")

	nextRec := settleWelcome(t, handler)
	require.Equal(t, http.StatusOK, nextRec.Code)
	require.Contains(t, nextRec.Body.String(), "Ledgers")

	recentPath := recentDirsPath(t)
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
	recentPath := recentDirsPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(recentPath), 0o700))
	require.NoError(t, os.WriteFile(recentPath, []byte(dir+"\n"), 0o600))

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
	require.Contains(t, openedRec.Body.String(), "Opening…")
	settled := settleWelcome(t, handler)
	require.Contains(t, settled.Body.String(), "contapila.cue")
	require.Contains(t, settled.Body.String(), "Choose a folder")
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
		require.Contains(t, rec.Body.String(), "Opening…")
		settled := settleWelcome(t, handler)
		require.Equal(t, http.StatusOK, settled.Code)
		require.Empty(t, settled.Header().Get("Location"))
		require.Contains(t, settled.Body.String(), "Choose a folder")
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
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "Opening…")
		settled := settleWelcome(t, handler)
		require.Equal(t, http.StatusSeeOther, settled.Code)
		loc, err := url.Parse(settled.Header().Get("Location"))
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
		require.Contains(t, rec.Body.String(), "Opening…")
		settled := settleWelcome(t, handler)
		require.Equal(t, http.StatusOK, settled.Code)
		require.Contains(t, settled.Body.String(), "portal broke")
		require.Contains(t, settled.Body.String(), "Choose a folder")
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
		require.Contains(t, rec.Body.String(), "Opening…")
		settled := settleWelcome(t, handler)
		require.Equal(t, http.StatusOK, settled.Code)
		require.Contains(t, settled.Body.String(), "contapila.cue")
		require.Contains(t, settled.Body.String(), "Choose a folder")
	})
}

func TestWelcomeBrowseHeadlessUsesDialog(t *testing.T) {
	example := exampleProject(t)
	isolateWelcome(t)
	t.Setenv("ELETROCROMO_NO_UI", "1")
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "inside"), 0o755))
	t.Chdir(root)
	called := false
	withChooser(t, func(context.Context) ([]string, error) {
		called = true
		return nil, driver.ErrUnavailable
	})
	handler := projectHandler(t.Context())
	rec := postForm(handler, "/welcome/browse", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "Opening…")
	settled := settleWelcome(t, handler)
	require.True(t, called)
	require.Equal(t, http.StatusSeeOther, settled.Code)
	loc, err := url.Parse(settled.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "/welcome/browse", loc.Path)
	require.Equal(t, "1", loc.Query().Get("note"))
	require.Equal(t, root, loc.Query().Get("at"))

	withChooser(t, func(context.Context) ([]string, error) {
		return []string{example}, nil
	})
	picked := projectHandler(t.Context())
	rec = postForm(picked, "/welcome/browse", nil)
	require.Contains(t, rec.Body.String(), "Opening…")
	settled = settleWelcome(t, picked)
	require.Contains(t, settled.Body.String(), "Ledgers")
}

func TestLoadProjectContentTree(t *testing.T) {
	example := exampleProject(t)
	filedialog.RegisterContent(func([]string) (fs.FS, error) {
		return os.DirFS(example), nil
	})
	t.Cleanup(func() { filedialog.RegisterContent(nil) })

	handler, err := loadProject(t.Context(), "content://tree")
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "Ledgers")
}

func TestWelcomeBrowseDoesNotWaitForDialog(t *testing.T) {
	isolateWelcome(t)
	release := make(chan struct{})
	started := make(chan struct{})
	var startOnce sync.Once
	var closeOnce sync.Once
	unblock := func() { closeOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	withChooser(t, func(ctx context.Context) ([]string, error) {
		startOnce.Do(func() { close(started) })
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return nil, filedialog.ErrCanceled
	})
	handler := projectHandler(t.Context())
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- postForm(handler, "/welcome/browse", nil)
	}()
	select {
	case rec := <-done:
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "Opening…")
	case <-time.After(2 * time.Second):
		require.FailNow(t, "browse handler blocked on the folder dialog")
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "folder dialog was not started after the response")
	}
	unblock()
	settled := settleWelcome(t, handler)
	require.Contains(t, settled.Body.String(), "Choose a folder")
}

func TestWelcomeOpenDoesNotWaitForLoad(t *testing.T) {
	isolateWelcome(t)
	release := make(chan struct{})
	started := make(chan struct{})
	var startOnce sync.Once
	var closeOnce sync.Once
	unblock := func() { closeOnce.Do(func() { close(release) }) }
	previous := loadProject
	loadProject = func(ctx context.Context, dir string) (http.Handler, error) {
		startOnce.Do(func() { close(started) })
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return nil, errPortalBroke
	}
	t.Cleanup(func() {
		loadProject = previous
		unblock()
	})
	handler := projectHandler(t.Context())
	dir := t.TempDir()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- postForm(handler, "/welcome/open", url.Values{"path": {dir}})
	}()
	select {
	case rec := <-done:
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "Opening…")
	case <-time.After(2 * time.Second):
		require.FailNow(t, "open handler blocked while the project was loading")
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "project load was not started after the response")
	}
	unblock()
	settled := settleWelcome(t, handler)
	require.Contains(t, settled.Body.String(), "portal broke")
	require.Contains(t, settled.Body.String(), "Choose a folder")
}
