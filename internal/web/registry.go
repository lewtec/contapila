package web

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"

	docsutil "github.com/lucasew/contapila-go/internal/docs"
)

// Registry / path sentinel errors.
var (
	ErrBuildPathDot = errors.New("web: refusing path with ..")
	ErrEmptyFileRel = errors.New("web: empty file path for instance")
)

// Kind classifies a concrete route instance for the static builder.
type Kind int

const (
	// KindPage is server-rendered HTML (written as …/index.html).
	KindPage Kind = iota
	// KindStatic is an embedded asset under /static/.
	KindStatic
	// KindDoc is a ledger document served under /docfile/.
	KindDoc
)

// Instance is one concrete GET path the site can serve or materialize.
type Instance struct {
	Path string // absolute URL path, e.g. /l/personal/check
	Kind Kind
}

// ExpandFunc yields concrete instances for one registered route pattern.
// Nil means the route is live-only (e.g. redirects) and skipped by build.
// sess is the same Session used for render (lazy OpenProject / OpenLedger).
type ExpandFunc func(sess *Session) ([]Instance, error)

// Route is one mux pattern plus optional static-site expansion.
type Route struct {
	Pattern string
	Handle  http.Handler
	Expand  ExpandFunc
}

// Registry is the shared route table for live web and contapila build.
type Registry struct {
	routes []Route
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register appends a route. Mount order follows registration order.
func (r *Registry) Register(rt Route) {
	if r == nil {
		return
	}
	r.routes = append(r.routes, rt)
}

// Routes returns registered routes in mount order (copy).
func (r *Registry) Routes() []Route {
	if r == nil {
		return nil
	}
	out := make([]Route, len(r.routes))
	copy(out, r.routes)
	return out
}

// Mount registers every route on mux. Patterns must be unique.
func (r *Registry) Mount(mux *http.ServeMux) {
	if r == nil || mux == nil {
		return
	}
	for _, rt := range r.routes {
		if rt.Pattern == "" || rt.Handle == nil {
			continue
		}
		mux.Handle(rt.Pattern, rt.Handle)
	}
}

// Instances expands all routes with Expand set. Order is registration order,
// then each expander's own order. Duplicate paths are not deduped.
func (r *Registry) Instances(sess *Session) ([]Instance, error) {
	if r == nil {
		return nil, nil
	}
	var out []Instance
	for _, rt := range r.routes {
		if rt.Expand == nil {
			continue
		}
		insts, err := rt.Expand(sess)
		if err != nil {
			return nil, err
		}
		out = append(out, insts...)
	}
	return out, nil
}

// ReportPages is the fixed ledger report set (sidebar + build).
func ReportPages() []string {
	return []string{"check", "balances", "journal", "pnl", "networth", "documents", "prices"}
}

// DefaultRegistry wires the built-in UI routes for s.
// Registration order matches the historical Handler() table (specific before {page}).
func DefaultRegistry(s *Server) *Registry {
	r := NewRegistry()
	registerStatic(r)
	registerDocFile(r, s)
	registerIndex(r, s)
	registerAccount(r, s)
	registerCommodity(r, s)
	registerLedgerPage(r, s)
	registerLedgerRoot(r, s)
	return r
}

func registerStatic(r *Registry) {
	r.Register(Route{
		Pattern: "GET /static/",
		Handle:  http.FileServer(http.FS(staticFS)),
		Expand: func(sess *Session) ([]Instance, error) {
			var out []Instance
			err := fs.WalkDir(staticFS, "static", func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				out = append(out, Instance{Path: "/" + path.Clean(p), Kind: KindStatic})
				return nil
			})
			if err != nil {
				return nil, err
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
			return out, nil
		},
	})
}

func registerDocFile(r *Registry, s *Server) {
	r.Register(Route{
		Pattern: "GET /docfile/{path...}",
		Handle:  http.HandlerFunc(s.handleDocFile),
		Expand: func(sess *Session) ([]Instance, error) {
			if sess == nil {
				return nil, ErrSessionNil
			}
			names, err := sess.LedgerNames()
			if err != nil {
				return nil, err
			}
			seen := map[string]struct{}{}
			var out []Instance
			for _, name := range names {
				l, err := sess.Ledger(name)
				if err != nil {
					return nil, err
				}
				for _, d := range l.Documents {
					p := strings.Trim(strings.ReplaceAll(d.Path, "\\", "/"), "/")
					p = strings.TrimPrefix(path.Clean("/"+p), "/")
					if p == "" || p == "." || !docsutil.IsLedgerDocPath(p) {
						continue
					}
					u := "/docfile/" + p
					if _, ok := seen[u]; ok {
						continue
					}
					seen[u] = struct{}{}
					out = append(out, Instance{Path: u, Kind: KindDoc})
				}
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
			return out, nil
		},
	})
}

func registerIndex(r *Registry, s *Server) {
	r.Register(Route{
		Pattern: "GET /{$}",
		Handle:  http.HandlerFunc(s.handleIndex),
		Expand: func(sess *Session) ([]Instance, error) {
			return []Instance{{Path: "/", Kind: KindPage}}, nil
		},
	})
}

func registerAccount(r *Registry, s *Server) {
	r.Register(Route{
		Pattern: "GET /l/{ledger}/account/{account...}",
		Handle:  http.HandlerFunc(s.handleAccount),
		Expand: func(sess *Session) ([]Instance, error) {
			if sess == nil {
				return nil, ErrSessionNil
			}
			names, err := sess.LedgerNames()
			if err != nil {
				return nil, err
			}
			var out []Instance
			for _, name := range names {
				l, err := sess.Ledger(name)
				if err != nil {
					return nil, err
				}
				accts := make([]string, 0, len(l.Accounts))
				for a := range l.Accounts {
					accts = append(accts, a)
				}
				sort.Strings(accts)
				for _, a := range accts {
					out = append(out, Instance{Path: accountURL(name, a, ""), Kind: KindPage})
				}
			}
			return out, nil
		},
	})
}

func registerCommodity(r *Registry, s *Server) {
	r.Register(Route{
		Pattern: "GET /l/{ledger}/commodity/{commodity...}",
		Handle:  http.HandlerFunc(s.handleCommodity),
		Expand: func(sess *Session) ([]Instance, error) {
			if sess == nil {
				return nil, ErrSessionNil
			}
			names, err := sess.LedgerNames()
			if err != nil {
				return nil, err
			}
			var out []Instance
			for _, name := range names {
				l, err := sess.Ledger(name)
				if err != nil {
					return nil, err
				}
				comms := make([]string, 0, len(l.Commodities))
				for c := range l.Commodities {
					comms = append(comms, c)
				}
				sort.Strings(comms)
				for _, c := range comms {
					out = append(out, Instance{Path: commodityURL(name, c, ""), Kind: KindPage})
				}
			}
			return out, nil
		},
	})
}

func registerLedgerPage(r *Registry, s *Server) {
	r.Register(Route{
		Pattern: "GET /l/{ledger}/{page}",
		Handle:  http.HandlerFunc(s.handleLedgerPage),
		Expand: func(sess *Session) ([]Instance, error) {
			if sess == nil {
				return nil, ErrSessionNil
			}
			names, err := sess.LedgerNames()
			if err != nil {
				return nil, err
			}
			pages := ReportPages()
			var out []Instance
			for _, name := range names {
				// Touch ledger so booking errors surface during expand, not mid-render.
				if _, err := sess.Ledger(name); err != nil {
					return nil, err
				}
				for _, page := range pages {
					out = append(out, Instance{Path: ledgerURL(name, page, ""), Kind: KindPage})
				}
			}
			return out, nil
		},
	})
}

func registerLedgerRoot(r *Registry, s *Server) {
	// Live: 302 → /l/{ledger}/check. Build: materialize l/{ledger}/index.html
	// (same body as check) so bare /l/{ledger}/ works on static hosts (rclone, etc.).
	r.Register(Route{
		Pattern: "GET /l/{ledger}/{$}",
		Handle: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "/l/"+req.PathValue("ledger")+"/check", http.StatusFound)
		}),
		Expand: func(sess *Session) ([]Instance, error) {
			if sess == nil {
				return nil, ErrSessionNil
			}
			names, err := sess.LedgerNames()
			if err != nil {
				return nil, err
			}
			var out []Instance
			for _, name := range names {
				// Trailing slash matches the mux pattern; fileRel → l/{name}/index.html.
				out = append(out, Instance{
					Path: "/l/" + url.PathEscape(name) + "/",
					Kind: KindPage,
				})
			}
			return out, nil
		},
	})
}

// fileRel maps a URL path to a path relative to the build output directory.
//
// HTML pages (except site root and trailing-slash dirs) are written as
// *.html files, e.g. /l/acme/check → l/acme/check.html. Live HTML still uses
// extensionless hrefs; build rewrites those links via rewriteStaticPageLinks
// so static hosts (rclone, etc.) fetch a real file with a text/html MIME type
// instead of relying on /path → /path/index.html directory indexes.
// Paths that end with / (ledger roots) use …/index.html so the path can also
// be a directory for child pages.
func fileRel(inst Instance) (string, error) {
	raw := strings.TrimPrefix(inst.Path, "/")
	// Preserve intent before Clean drops a trailing slash.
	dirIndex := inst.Path != "/" && strings.HasSuffix(inst.Path, "/")
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("path unescape %q: %w", inst.Path, err)
	}
	decoded = path.Clean("/" + decoded)
	decoded = strings.TrimPrefix(decoded, "/")
	if strings.Contains(decoded, "..") {
		return "", fmt.Errorf("%w: %q", ErrBuildPathDot, inst.Path)
	}
	switch inst.Kind {
	case KindPage:
		if decoded == "" || decoded == "." {
			return "index.html", nil
		}
		if dirIndex {
			return path.Join(decoded, "index.html"), nil
		}
		return decoded + ".html", nil
	default:
		if decoded == "" || decoded == "." {
			return "", fmt.Errorf("%w: %q", ErrEmptyFileRel, inst.Path)
		}
		return decoded, nil
	}
}
