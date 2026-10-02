package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/lewtec/lewkit/x/driver"
	"github.com/lewtec/lewkit/x/driver/filedialog"
	_ "github.com/lewtec/lewkit/x/driver/filedialog/prelude" // registers folder dialog drivers
	lewpath "github.com/lewtec/lewkit/x/path"
)

const (
	welcomeDirLimit = 200
	recentDirLimit  = 8
)

// chooseFolder asks the OS for one directory. Tests replace it.
// The Linux driver talks to the portal from the caller's goroutine.
var chooseFolder = func(ctx context.Context) ([]string, error) {
	return filedialog.Choose(ctx, filedialog.Request{
		Title:  "Open folder",
		Folder: true,
	})
}

// projectGate serves the ledger UI once a project is open.
// The first request tries the working directory, then the app data
// directory. Until that works, the page asks for a folder. A missed
// auto-open does not stick: a later choice can still open a project.
// The headless host must serve that page from the loopback server.
// Returning the miss as a process error deadlocks the Android splash.
type projectGate struct {
	ctx     context.Context
	once    sync.Once
	mu      sync.Mutex
	handler http.Handler
	openErr error
}

func projectHandler(ctx context.Context) http.Handler {
	return &projectGate{ctx: ctx}
}

func (g *projectGate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.ensureTried()
	if handler := g.current(); handler != nil {
		handler.ServeHTTP(w, r)
		return
	}
	g.serveIdle(w, r)
}

func (g *projectGate) ensureTried() {
	g.once.Do(func() {
		handler, err := openHeadlessProject(g.ctx)
		g.mu.Lock()
		defer g.mu.Unlock()
		if err != nil || handler == nil {
			if err == nil {
				err = errors.New("could not open a project")
			}
			g.openErr = err
			return
		}
		if g.handler == nil {
			g.handler = handler
		}
	})
}

func (g *projectGate) current() http.Handler {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.handler
}

func (g *projectGate) adopt(handler http.Handler) {
	g.mu.Lock()
	g.handler = handler
	g.mu.Unlock()
}

func (g *projectGate) startupError() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.openErr == nil {
		return ""
	}
	return g.openErr.Error()
}

func (g *projectGate) serveIdle(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/", "":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		g.render(w, welcomeView{})
	case "/welcome/browse":
		switch r.Method {
		case http.MethodPost:
			g.browseWithDialog(w, r)
		case http.MethodGet, http.MethodHead:
			g.serveBrowse(w, r)
		default:
			methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
		}
	case "/welcome/open":
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodPost:
			g.openFromRequest(w, r)
		default:
			methodNotAllowed(w, http.MethodPost)
		}
	default:
		http.NotFound(w, r)
	}
}

func (g *projectGate) browseWithDialog(w http.ResponseWriter, r *http.Request) {
	paths, err := chooseFolder(r.Context())
	if err == nil {
		if len(paths) == 0 {
			g.render(w, welcomeView{})
			return
		}
		g.openDirectory(w, r, paths[0])
		return
	}
	if errors.Is(err, filedialog.ErrCanceled) {
		g.render(w, welcomeView{})
		return
	}
	if errors.Is(err, driver.ErrUnavailable) {
		if at := browseSeed(); at != "" {
			query := url.Values{}
			query.Set("at", at)
			query.Set("note", "1")
			redirectWelcome(w, r, "/welcome/browse?"+query.Encode())
			return
		}
		g.render(w, welcomeView{DialogNote: true})
		return
	}
	g.render(w, welcomeView{Error: err.Error()})
}

func (g *projectGate) serveBrowse(w http.ResponseWriter, r *http.Request) {
	note := r.URL.Query().Get("note") == "1"
	at, ok := cleanWelcomeDir(r.URL.Query().Get("at"))
	if !ok {
		at = browseSeed()
		ok = at != ""
	}
	if !ok {
		g.render(w, welcomeView{DialogNote: true, Error: "could not list a folder"})
		return
	}
	dirs, err := listChildDirs(at)
	view := welcomeView{DialogNote: note, At: at}
	if parent := parentDir(at); parent != "" {
		view.Up = welcomeLink{Label: "Up", Href: browseHref(parent, note)}
	}
	for _, dir := range dirs {
		view.Dirs = append(view.Dirs, welcomeLink{
			Label: filepath.Base(dir),
			Href:  browseHref(dir, note),
		})
	}
	if err != nil {
		view.Error = err.Error()
	}
	g.render(w, view)
}

func (g *projectGate) openFromRequest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		g.render(w, welcomeView{Error: "could not read the folder"})
		return
	}
	g.openDirectory(w, r, r.FormValue("path"))
}

func (g *projectGate) openDirectory(w http.ResponseWriter, r *http.Request, dir string) {
	cleaned, ok := cleanWelcomeDir(dir)
	if !ok {
		g.render(w, welcomeView{Error: "not a folder"})
		return
	}
	handler, err := desktopHandler(g.ctx, cleaned, nil)
	if err != nil {
		g.render(w, welcomeView{Error: err.Error()})
		return
	}
	rememberBestEffort(cleaned)
	g.adopt(handler)
	redirectWelcome(w, r, "/")
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func redirectWelcome(w http.ResponseWriter, r *http.Request, target string) {
	if r != nil && r.URL != nil && r.URL.Scheme != "" {
		if ref, err := url.Parse(target); err == nil {
			target = r.URL.ResolveReference(ref).String()
		}
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

type welcomeLink struct {
	Label string
	Href  string
}

type welcomeView struct {
	Error      string
	DialogNote bool
	At         string
	Up         welcomeLink
	Dirs       []welcomeLink
	Recent     []welcomeLink
	Browse     welcomeLink
}

func (g *projectGate) render(w http.ResponseWriter, view welcomeView) {
	if view.At == "" && view.Error == "" {
		if message := g.startupError(); message != "" {
			view.Error = message
		}
	}
	if view.At == "" && view.Browse.Href == "" {
		if at := browseSeed(); at != "" {
			view.Browse = welcomeLink{Label: "Browse this computer", Href: browseHref(at, false)}
		}
	}
	for _, dir := range readRecentDirs() {
		view.Recent = append(view.Recent, welcomeLink{Label: dir, Href: openHref(dir)})
	}
	var buf bytes.Buffer
	if err := welcomeTemplate.Execute(&buf, view); err != nil {
		http.Error(w, "welcome page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := buf.WriteTo(w); err != nil {
		return
	}
}

func browseHref(dir string, note bool) string {
	query := url.Values{}
	query.Set("at", dir)
	if note {
		query.Set("note", "1")
	}
	return "/welcome/browse?" + query.Encode()
}

func openHref(dir string) string {
	query := url.Values{}
	query.Set("path", dir)
	return "/welcome/open?" + query.Encode()
}

func browseSeed() string {
	var seeds []string
	if cwd, err := os.Getwd(); err == nil {
		seeds = append(seeds, cwd)
	}
	if home, err := os.UserHomeDir(); err == nil {
		seeds = append(seeds, home)
	}
	seeds = append(seeds, "/storage/emulated/0", "/sdcard", "/storage")
	for _, seed := range seeds {
		cleaned, ok := cleanWelcomeDir(seed)
		if !ok || parentDir(cleaned) == "" {
			continue
		}
		if _, err := os.ReadDir(cleaned); err != nil {
			continue
		}
		return cleaned
	}
	return ""
}

func listChildDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(dir, name)
		info, statErr := os.Stat(full)
		if statErr != nil || !info.IsDir() {
			continue
		}
		dirs = append(dirs, full)
		if len(dirs) == welcomeDirLimit {
			break
		}
	}
	slices.SortFunc(dirs, func(a, b string) int {
		return cmp.Or(
			strings.Compare(strings.ToLower(filepath.Base(a)), strings.ToLower(filepath.Base(b))),
			strings.Compare(a, b),
		)
	})
	return dirs, nil
}

func parentDir(dir string) string {
	parent := filepath.Clean(filepath.Dir(dir))
	if parent == filepath.Clean(dir) {
		return ""
	}
	return parent
}

// cleanWelcomeDir matches the lewkit recent-dirs rule: a trimmed absolute
// path that exists as a directory. Relative paths are rejected.
func cleanWelcomeDir(dir string) (string, bool) {
	dir = strings.TrimSpace(dir)
	if dir == "" || strings.ContainsAny(dir, "\r\n") || !filepath.IsAbs(dir) {
		return "", false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	root, err := lewpath.Open(abs)
	if err != nil {
		return "", false
	}
	root.Close()
	return abs, true
}

func recentFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lewkit", "recent-dirs"), nil
}

func readRecentDirs() []string {
	file, err := recentFile()
	if err != nil {
		return nil
	}
	text, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for line := range strings.SplitSeq(string(text), "\n") {
		dir, ok := cleanWelcomeDir(line)
		if !ok || seen[dir] {
			continue
		}
		seen[dir] = true
		out = append(out, dir)
		if len(out) == recentDirLimit {
			break
		}
	}
	return out
}

func rememberBestEffort(dir string) {
	cleaned, ok := cleanWelcomeDir(dir)
	if !ok {
		return
	}
	file, err := recentFile()
	if err != nil {
		return
	}
	lines := make([]string, 0, recentDirLimit)
	lines = append(lines, cleaned)
	for _, item := range readRecentDirs() {
		if item == cleaned || len(lines) == recentDirLimit {
			continue
		}
		lines = append(lines, item)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return
	}
	if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return
	}
}

var welcomeTemplate = template.Must(template.New("welcome").Parse(welcomeSource))

const welcomeSource = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Contapila</title>
<style>
:root {
  color-scheme: light dark;
  --bg: #f3f6f3;
  --text: #1c2b24;
  --muted: #5c6f64;
  --line: #d5e0d8;
  --accent: #1f6b45;
  --on-accent: #f4fff8;
  --danger: #8f2d2d;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #121a16;
    --text: #e7f0ea;
    --muted: #a3b5aa;
    --line: #314038;
    --accent: #7dcea0;
    --on-accent: #102018;
    --danger: #ffb4b4;
  }
}
body {
  margin: 0;
  font: 16px/1.45 system-ui, sans-serif;
  background: var(--bg);
  color: var(--text);
}
main {
  max-width: 32rem;
  margin: 0 auto;
  padding: 1.5rem 1rem 3rem;
}
h1 { font-size: 1.6rem; margin: 0 0 0.25rem; }
h2 { font-size: 0.8rem; letter-spacing: 0.04em; text-transform: uppercase; color: var(--muted); margin: 1.5rem 0 0.25rem; }
p { margin: 0.4rem 0; }
.lead, .note, .path { color: var(--muted); }
.path { font-family: ui-monospace, monospace; font-size: 0.85rem; word-break: break-all; }
.error { color: var(--danger); }
ul { list-style: none; padding: 0; margin: 0.5rem 0 0; }
li { border-top: 1px solid var(--line); }
a { color: inherit; text-decoration: none; }
li a { display: block; padding: 0.85rem 0.15rem; }
button {
  font: inherit;
  width: 100%;
  margin-top: 0.75rem;
  padding: 0.8rem 1rem;
  border: 0;
  border-radius: 0.7rem;
  background: var(--accent);
  color: var(--on-accent);
}
:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
</style>
</head>
<body>
<main>
<h1>Contapila</h1>
{{if .At}}
<p class="path">{{.At}}</p>
{{if .Up.Href}}<p><a href="{{.Up.Href}}">{{.Up.Label}}</a></p>{{end}}
<form method="post" action="/welcome/open">
<input type="hidden" name="path" value="{{.At}}">
<button type="submit">Open this folder</button>
</form>
{{if .Error}}<p class="error">{{.Error}}</p>{{end}}
{{if .DialogNote}}<p class="note">No folder dialog on this system.</p>{{end}}
{{if .Dirs}}
<ul>
{{range .Dirs}}<li><a href="{{.Href}}">{{.Label}}</a></li>{{end}}
</ul>
{{else}}
<p class="note">No folders here.</p>
{{end}}
{{else}}
<p class="lead">Choose a folder</p>
{{if .Error}}<p class="error">{{.Error}}</p>{{end}}
{{if .DialogNote}}<p class="note">No folder dialog on this system.</p>{{end}}
<form method="post" action="/welcome/browse">
<button type="submit">Open a folder</button>
</form>
{{if .Browse.Href}}<p><a href="{{.Browse.Href}}">{{.Browse.Label}}</a></p>{{end}}
{{end}}
{{if .Recent}}
<h2>Recent</h2>
<ul>
{{range .Recent}}<li><a href="{{.Href}}">{{.Label}}</a></li>{{end}}
</ul>
{{end}}
</main>
</body>
</html>
`
