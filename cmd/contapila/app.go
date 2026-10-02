package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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
// When the working directory is not a project, the window asks for a folder.
func runApp(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM)
	defer stop()
	// The packaged host waits for ELETROCROMO_READY. Returning a project
	// error before app.App.Run skips that line. Android then reports the
	// failure after the UI loop has stopped, and the splash never moves.
	handler := projectHandler(ctx)
	if headlessHost() {
		return app.App{
			Title:   "Contapila",
			Width:   desktopWidth,
			Height:  desktopHeight,
			Handler: app.Web(handler),
		}.Run(ctx)
	}
	profile, err := windowProfile(ctx)
	if err != nil {
		return err
	}
	return openDesktopView(ctx, handler, profile)
}

// windowProfile is the web view storage directory. A stamped bundle wins.
// Otherwise the desktop command's profile directory is used.
func windowProfile(ctx context.Context) (string, error) {
	root, err := bundle.Resolve(ctx)
	if err == nil && root.Profile != "" {
		return root.Profile, nil
	}
	return desktopProfileDir()
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
