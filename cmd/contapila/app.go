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
	"github.com/lewtec/lewkit/x/driver/bundle"
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

// runApp opens the stamped UI the same way as the lewkit welcome example.
// The folder window is app.GUI. After it returns a project, the ledger is
// app.Web. Lewkit opens that as a web view, or as loopback when the host
// asked for no UI.
//
// Android starts runApp on the loader thread with no session. One entry.Run
// keeps that loop alive for both windows. A welcome session that owns the
// loop stops it on return, and the content-provider reads that follow have
// no thread left to call Java on. The surface then stays blank.
func runApp(ctx context.Context) error {
	if taskgroup.FromContext(ctx) != nil {
		return runAppBody(ctx)
	}
	return entry.Run(ctx, runAppBody)
}

func runAppBody(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM)
	defer stop()
	handler, err := interactiveHandler(ctx)
	if err != nil || handler == nil {
		return err
	}
	return app.App{
		Title:   "Contapila",
		Width:   desktopWidth,
		Height:  desktopHeight,
		Handler: app.Web(handler),
	}.Run(ctx)
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
	err = app.App{
		Title:   "Contapila",
		Width:   880,
		Height:  720,
		Handler: app.GUI(model),
	}.Run(ctx)
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
		if !strings.HasPrefix(dir, "content:") {
			if err := gui.Remember(dir); err != nil {
				return nil, err
			}
		}
		return handler, nil
	}
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
