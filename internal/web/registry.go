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
	"github.com/lucasew/contapila-go/internal/engine"
	"github.com/lucasew/contapila-go/internal/prices"
	"github.com/lucasew/contapila-go/pkg/project"
)

// Registry / path sentinel errors.
var (
	ErrSiteCtxNil   = errors.New("web: site context is nil")
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

// SiteCtx is project state used when expanding parameterized routes for build.
type SiteCtx struct {
	Root    string
	Project *project.Project
	Prices  *prices.DB

	ledgers map[string]*engine.Ledger
}

// NewSiteCtx opens the project at root for route expansion.
func NewSiteCtx(root string) (*SiteCtx, error) {
	p, pdb, _, err := engine.OpenProject(root)
	if err != nil {
		return nil, err
	}
	return &SiteCtx{Root: root, Project: p, Prices: pdb}, nil
}

// Ledger returns a cached booked ledger by directory name.
func (c *SiteCtx) Ledger(name string) (*engine.Ledger, error) {
	if c == nil || c.Project == nil {
		return nil, ErrSiteCtxNil
	}
	if c.ledgers == nil {
		c.ledgers = make(map[string]*engine.Ledger)
	}
	if l, ok := c.ledgers[name]; ok {
		return l, nil
	}
	l, err := engine.OpenLedger(c.Project, c.Prices, name)
	if err != nil {
		return nil, err
	}
	c.ledgers[name] = l
	return l, nil
}

// ExpandFunc yields concrete instances for one registered route pattern.
// Nil means the route is live-only (e.g. redirects) and skipped by build.
type ExpandFunc func(ctx *SiteCtx) ([]Instance, error)

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
func (r *Registry) Instances(ctx *SiteCtx) ([]Instance, error) {
	if r == nil {
		return nil, nil
	}
	var out []Instance
	for _, rt := range r.routes {
		if rt.Expand == nil {
			continue
		}
		insts, err := rt.Expand(ctx)
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
		Expand: func(ctx *SiteCtx) ([]Instance, error) {
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
		Expand: func(ctx *SiteCtx) ([]Instance, error) {
			if ctx == nil || ctx.Project == nil {
				return nil, nil
			}
			seen := map[string]struct{}{}
			var out []Instance
			for _, name := range engine.LedgerNames(ctx.Project) {
				l, err := ctx.Ledger(name)
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
		Expand: func(ctx *SiteCtx) ([]Instance, error) {
			return []Instance{{Path: "/", Kind: KindPage}}, nil
		},
	})
}

func registerAccount(r *Registry, s *Server) {
	r.Register(Route{
		Pattern: "GET /l/{ledger}/account/{account...}",
		Handle:  http.HandlerFunc(s.handleAccount),
		Expand: func(ctx *SiteCtx) ([]Instance, error) {
			if ctx == nil || ctx.Project == nil {
				return nil, nil
			}
			var out []Instance
			for _, name := range engine.LedgerNames(ctx.Project) {
				l, err := ctx.Ledger(name)
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
		Expand: func(ctx *SiteCtx) ([]Instance, error) {
			if ctx == nil || ctx.Project == nil {
				return nil, nil
			}
			var out []Instance
			for _, name := range engine.LedgerNames(ctx.Project) {
				l, err := ctx.Ledger(name)
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
		Expand: func(ctx *SiteCtx) ([]Instance, error) {
			if ctx == nil || ctx.Project == nil {
				return nil, nil
			}
			pages := ReportPages()
			var out []Instance
			for _, name := range engine.LedgerNames(ctx.Project) {
				// Touch ledger so booking errors surface during expand, not mid-render.
				if _, err := ctx.Ledger(name); err != nil {
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
	// Live redirect only; build emits /l/{ledger}/check instead.
	r.Register(Route{
		Pattern: "GET /l/{ledger}/{$}",
		Handle: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "/l/"+req.PathValue("ledger")+"/check", http.StatusFound)
		}),
		Expand: nil,
	})
}

// fileRel maps a URL path to a path relative to the build output directory.
func fileRel(inst Instance) (string, error) {
	raw := strings.TrimPrefix(inst.Path, "/")
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
		return path.Join(decoded, "index.html"), nil
	default:
		if decoded == "" || decoded == "." {
			return "", fmt.Errorf("%w: %q", ErrEmptyFileRel, inst.Path)
		}
		return decoded, nil
	}
}
