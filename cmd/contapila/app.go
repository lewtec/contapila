package main

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/lewtec/lewkit/x/app"
	"github.com/lewtec/lewkit/x/driver/bundle"
	"github.com/lewtec/lewkit/x/entry"
	"github.com/lewtec/lewkit/x/release"
)

func init() { entry.Bind(runApp) }

// wantWindow reports whether this process should open the desktop UI.
// lewkit release run stamps the app id and the version, then starts the
// binary with no arguments. Any argument stays on the CLI, including desktop.
func wantWindow(args []string) bool {
	if len(args) > 0 {
		return false
	}
	return releaseStamped()
}

func releaseStamped() bool {
	if !stampedVersion(release.Version()) {
		return false
	}
	_, err := release.AppID()
	return err == nil
}

func stampedVersion(version string) bool {
	version = strings.TrimSpace(version)
	return version != "" && version != "dev" && !strings.HasPrefix(version, "dev-")
}

func headlessHost() bool {
	return envOn("LEWKIT_NO_UI") || envOn("ELETROCROMO_NO_UI")
}

func envOn(key string) bool {
	value := strings.TrimSpace(os.Getenv(key))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

// runApp opens the read-only UI for a stamped binary.
// A headless host serves the handler on a loopback port. Otherwise the
// OS web view opens it, and a missing web view returns an error.
func runApp(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM)
	defer stop()
	// The packaged host waits for ELETROCROMO_READY. Returning a project
	// error before app.App.Run skips that line. Android then reports the
	// failure after the UI loop has stopped, and the splash never moves.
	if headlessHost() {
		return app.App{
			Title:   "Contapila",
			Width:   desktopWidth,
			Height:  desktopHeight,
			Handler: app.Web(lazyProjectHandler(ctx)),
		}.Run(ctx)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	handler, err := desktopHandler(ctx, cwd, nil)
	if err != nil {
		return err
	}
	root, err := bundle.Resolve(ctx)
	if err != nil {
		return err
	}
	return openDesktopView(ctx, handler, root.Profile)
}

// lazyProjectHandler opens the project on the first request.
// The loopback server is already listening by then, so a missing project
// is a page instead of a process error.
func lazyProjectHandler(ctx context.Context) http.Handler {
	var (
		once    sync.Once
		handler http.Handler
		openErr error
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			handler, openErr = openHeadlessProject(ctx)
		})
		if openErr != nil {
			writeStartupPage(w, openErr)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func openHeadlessProject(ctx context.Context) (http.Handler, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}
	handler, openErr := desktopHandler(ctx, cwd, nil)
	if openErr == nil {
		return handler, nil
	}
	root, err := bundle.Resolve(ctx)
	if err == nil && root.Data != "" && filepath.Clean(root.Data) != filepath.Clean(cwd) {
		if handler, err := desktopHandler(ctx, root.Data, nil); err == nil {
			return handler, nil
		}
	}
	return nil, openErr
}

func writeStartupPage(w http.ResponseWriter, err error) {
	msg := "could not open a project"
	if err != nil {
		msg = err.Error()
	}
	body := "<!doctype html><meta charset=utf-8><meta name=viewport content=\"width=device-width,initial-scale=1\"><title>Contapila</title><h1>Contapila</h1><p>" + html.EscapeString(msg) + "</p>"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, body); err != nil {
		return
	}
}
