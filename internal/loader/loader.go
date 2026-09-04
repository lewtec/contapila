package loader

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
	"github.com/lucasew/contapila-go/internal/filesys"
	"github.com/lucasew/contapila-go/internal/parser"
	"github.com/lucasew/contapila-go/internal/source"
)

// Sentinel errors for include expansion (also reported via diags).
var (
	ErrIncludeCycle     = errors.New("include cycle")
	ErrIncludePathEmpty = errors.New("include path is empty")
	ErrIncludeMissing   = errors.New("include missing")
	ErrIncludeIsDir     = errors.New("include is a directory")
	ErrNilContext       = errors.New("nil context")
)

// LoadFile parses a file and expands includes depth-first (disk).
func LoadFile(ctx context.Context, path string) ([]ast.Directive, diag.List, error) {
	return LoadFileFS(ctx, filesys.OS{}, path)
}

// LoadFileFS is LoadFile using fsys for reads/stats.
func LoadFileFS(ctx context.Context, fsys filesys.FS, path string) ([]ast.Directive, diag.List, error) {
	if ctx == nil {
		return nil, nil, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if fsys == nil {
		fsys = filesys.OS{}
	}
	var diags diag.List
	seen := map[string]bool{}
	stack := map[string]bool{}
	var out []ast.Directive
	err := loadOne(ctx, fsys, path, &out, &diags, seen, stack)
	return out, diags, err
}

func loadOne(ctx context.Context, fsys filesys.FS, path string, out *[]ast.Directive, diags *diag.List, seen, stack map[string]bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// file may not exist yet for eval; use abs
		real = abs
	}
	if stack[real] {
		diags.Error(path, 0, "include cycle detected")
		return fmt.Errorf("%w at %s", ErrIncludeCycle, path)
	}
	if seen[real] {
		return nil // dedupe
	}
	stack[real] = true
	defer delete(stack, real)

	f, err := source.NewFS(fsys, abs)
	if err != nil {
		return err
	}
	seen[real] = true

	dirs, pdiags, err := parser.ParseFile(f)
	diags.Merge(pdiags)
	if err != nil {
		return err
	}

	dir := filepath.Dir(abs)
	for _, d := range dirs {
		inc, ok := d.(ast.Include)
		if !ok {
			*out = append(*out, d)
			continue
		}
		if err := expandInclude(ctx, fsys, dir, inc.Path, out, diags, seen, stack); err != nil {
			return err
		}
	}
	return nil
}

func expandInclude(ctx context.Context, fsys filesys.FS, baseDir, pattern string, out *[]ast.Directive, diags *diag.List, seen, stack map[string]bool) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		diags.Error(baseDir, 0, "include path is empty")
		return ErrIncludePathEmpty
	}
	target := pattern
	if !filepath.IsAbs(pattern) {
		target = filepath.Join(baseDir, pattern)
	}

	if !hasGlob(pattern) {
		info, err := fsys.Stat(target)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				diags.Error(baseDir, 0, fmt.Sprintf("include missing: %s", pattern))
				return fmt.Errorf("%w: %s", ErrIncludeMissing, pattern)
			}
			return err
		}
		if info.IsDir() {
			diags.Error(baseDir, 0, fmt.Sprintf("include is a directory: %s", pattern))
			return fmt.Errorf("%w: %s", ErrIncludeIsDir, pattern)
		}
		return loadOne(ctx, fsys, target, out, diags, seen, stack)
	}

	// Glob still uses the process FS (overlay files without disk names won't appear).
	matches, err := filepath.Glob(target)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		diags.Warn(baseDir, 0, fmt.Sprintf("include glob matched zero files: %s", pattern))
		return nil
	}
	sort.Strings(matches)
	for _, m := range matches {
		info, err := fsys.Stat(m)
		if err != nil {
			// fall back to os for disk-only glob hits
			info, err = os.Stat(m)
			if err != nil {
				return err
			}
		}
		if info.IsDir() {
			continue
		}
		if err := loadOne(ctx, fsys, m, out, diags, seen, stack); err != nil {
			return err
		}
	}
	return nil
}

func hasGlob(s string) bool {
	for _, c := range s {
		if c == '*' || c == '?' || c == '[' {
			return true
		}
	}
	return false
}
