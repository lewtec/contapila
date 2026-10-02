package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/lewtec/lewkit/x/app"
	"github.com/lewtec/lewkit/x/driver"
	"github.com/lewtec/lewkit/x/driver/bundle"
	"github.com/lewtec/lewkit/x/driver/window"
	_ "github.com/lewtec/lewkit/x/driver/window/prelude" // registers the host window
	"github.com/lewtec/lewkit/x/entry"
	"github.com/lewtec/lewkit/x/release"
	"github.com/lewtec/lewkit/x/taskgroup"
	"github.com/lewtec/lewkit/x/ui/gui"
	"github.com/lucasew/contapila-go/internal/web"
	"github.com/lucasew/contapila-go/pkg/project"
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
// With no project, the lewkit folder window is the only window. It closes
// before the ledger web view opens on the picked folder.
// A packaged host still serves loopback so its outer window can load the
// ledger. That serve starts after the folder window closes.
// A process with no host window keeps the in-page folder list.
// A missing web view on a windowed launch returns an error and does not listen.
func runApp(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM)
	defer stop()
	// The packaged host waits for ELETROCROMO_READY. Returning a project
	// error before app.App.Run skips that line. Android then reports the
	// failure after the UI loop has stopped, and the splash never moves.
	if headlessHost() {
		return runHosted(ctx)
	}
	profile, err := windowProfile(ctx)
	if err != nil {
		return err
	}
	// Load the project before the web view. On macOS the first page runs
	// on the main thread until the response is written.
	handler, err := interactiveHandler(ctx)
	if err != nil || handler == nil {
		return err
	}
	return openDesktopView(ctx, handler, profile)
}

// runHosted serves the ledger on loopback for a packaged host.
// The folder window runs to completion first. The web view opens after it
// closes, already on the picked folder.
func runHosted(ctx context.Context) error {
	// Android calls runApp before the UI loop is pumping. The folder window
	// needs that loop, and a project error returned after it stops deadlocks
	// the host. Enter the loop here, then publish the folder result, then serve.
	open := func(ctx context.Context) error {
		gate := projectHandler(ctx)
		if chooseHosted(ctx, gate) {
			entry.NotifyFail("folder window closed")
			return nil
		}
		return app.App{
			Title:   "Contapila",
			Width:   desktopWidth,
			Height:  desktopHeight,
			Handler: app.Web(gate),
		}.Run(ctx)
	}
	if taskgroup.FromContext(ctx) != nil {
		return open(ctx)
	}
	return entry.Run(ctx, open)
}

// hostWindow reports whether a real host window can open.
// The in-memory window is always compatible and has nobody to click it.
var hostWindow = func(ctx context.Context) bool {
	handles, err := driver.List[window.Driver](ctx)
	if err != nil {
		return false
	}
	for _, handle := range handles {
		if handle.ID != "window_mem" {
			return true
		}
	}
	return false
}

// chooseHosted opens the folder window before any web view exists.
// It reports whether the user closed that window, so the web view stays shut.
func chooseHosted(ctx context.Context, gate *projectGate) bool {
	if !hostWindow(ctx) {
		return false
	}
	handler, err := interactiveHandler(ctx)
	return gate.applyWelcome(handler, err)
}

// interactiveHandler opens the ledger.
// A project in the working directory, or in the app data directory, opens
// directly. Otherwise the lewkit folder window picks one. A nil handler
// means the user closed that window.
func interactiveHandler(ctx context.Context) (http.Handler, error) {
	handler, err := openHeadlessProject(ctx)
	if err == nil {
		return handler, nil
	}
	if !errors.Is(err, project.ErrNotAProject) {
		return nil, err
	}
	return pickProject(ctx)
}

// welcomeOpener shows the folder window.
// An empty path and a nil error means the user closed it.
type welcomeOpener func(ctx context.Context, title string, dirs []gui.Directory) (string, error)

// openWelcome is the standard folder window. Tests replace it.
var openWelcome welcomeOpener = func(ctx context.Context, title string, dirs []gui.Directory) (string, error) {
	// A nil Logo is the LEWTEC lockup.
	logo, err := web.Logo()
	if err != nil {
		return "", err
	}
	model := gui.NewWelcome(gui.WelcomeArgs{Title: title, Dirs: dirs, Logo: logo})
	err = gui.Open(ctx, model, gui.Options{
		Config: window.Config{Title: "Contapila", Width: 880, Height: 720},
	})
	if path := model.Picked(); path != "" {
		return path, nil
	}
	if ctx.Err() != nil {
		return "", context.Cause(ctx)
	}
	if err != nil {
		return "", err
	}
	return "", nil
}

// pickProject shows the lewkit folder window until the user picks a project or closes it.
// EnsureDir is the wrong call: on a terminal it returns the working directory
// and never shows the window. release run keeps stdin a TTY.
// A folder that is not a project brings the window back with the reason.
func pickProject(ctx context.Context) (http.Handler, error) {
	dirs, err := gui.Recent()
	if err != nil {
		dirs = nil
	}
	title := "Contapila"
	for {
		if err := ctx.Err(); err != nil {
			return nil, context.Cause(ctx)
		}
		dir, err := openWelcome(ctx, title, dirs)
		if err != nil {
			return nil, err
		}
		if dir == "" {
			return nil, nil
		}
		handler, err := loadProject(ctx, dir)
		if err != nil {
			title = err.Error()
			continue
		}
		if err := gui.Remember(dir); err != nil {
			return nil, err
		}
		return handler, nil
	}
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
