package ingest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/parser"
)

// Sentinel errors for Apply / WriteAtomic.
var (
	ErrParseFailed       = errors.New("parse")
	ErrMissingSourceSpan = errors.New("directive missing source span")
	ErrInvalidSpan       = errors.New("invalid span")
)

// Apply merges incoming directives into the target beancount file text using
// source-span surgery. Directives without ingest_id are appended.
// Directives with ingest_id replace an existing directive that has the same
// metadata ingest_id, or are appended if none exists.
// Non-ingest regions of the file are preserved byte-for-byte.
//
// Returns the new file contents. Does not write to disk.
func Apply(fileText string, filename string, incoming []ast.Directive) (string, error) {
	// Index existing spans by ingest_id (last wins if duplicates in file).
	type span struct{ start, end int }
	idSpan := map[string]span{}
	if strings.TrimSpace(fileText) != "" {
		dirs, diags, err := parser.Parse(filename, []byte(fileText))
		if err != nil {
			return "", err
		}
		if diags.HasErrors() {
			return "", fmt.Errorf("%w %s: %s", ErrParseFailed, filename, diags.FormatErrors())
		}
		for _, d := range dirs {
			md := ast.DirectiveMetadata(d)
			if md == nil {
				continue
			}
			id := md[ast.IngestIDMetaKey]
			if id == "" {
				continue
			}
			meta := directiveMeta(d)
			if meta.EndByte <= meta.StartByte {
				return "", fmt.Errorf("%w for ingest_id %q", ErrMissingSourceSpan, id)
			}
			idSpan[id] = span{meta.StartByte, meta.EndByte}
		}
	}

	type repl struct {
		start, end int
		text       string
	}
	var replacements []repl
	var appends []string
	replaced := map[string]bool{}

	for _, d := range incoming {
		text, err := FormatDirective(d)
		if err != nil {
			return "", err
		}
		md := ast.DirectiveMetadata(d)
		id := ""
		if md != nil {
			id = md[ast.IngestIDMetaKey]
		}
		if id == "" {
			appends = append(appends, text)
			continue
		}
		if sp, ok := idSpan[id]; ok {
			replacements = append(replacements, repl{sp.start, sp.end, text})
			replaced[id] = true
			continue
		}
		appends = append(appends, text)
	}

	// Apply replacements from end to start so offsets stay valid.
	sort.Slice(replacements, func(i, j int) bool {
		return replacements[i].start > replacements[j].start
	})
	out := fileText
	for _, r := range replacements {
		if r.start < 0 || r.end > len(out) || r.start > r.end {
			return "", fmt.Errorf("%w [%d,%d) in file len %d", ErrInvalidSpan, r.start, r.end, len(out))
		}
		// Expand end to include a trailing newline if the original had one.
		end := r.end
		if end < len(out) && out[end] == '\n' {
			end++
		}
		// Ensure replacement ends with newline
		rep := r.text
		if !strings.HasSuffix(rep, "\n") {
			rep += "\n"
		}
		out = out[:r.start] + rep + out[end:]
	}

	if len(appends) > 0 {
		var b strings.Builder
		b.WriteString(out)
		if len(out) > 0 && !strings.HasSuffix(out, "\n") {
			b.WriteByte('\n')
		}
		if len(out) > 0 && !strings.HasSuffix(out, "\n\n") {
			// single blank line before ingest block if file non-empty
			if strings.HasSuffix(out, "\n") && !strings.HasSuffix(out, "\n\n") {
				// keep single newline between last line and append
			}
		}
		for _, a := range appends {
			b.WriteString(a)
		}
		out = b.String()
	}
	return out, nil
}

func directiveMeta(d ast.Directive) ast.Meta {
	switch v := d.(type) {
	case ast.Option:
		return v.Meta
	case ast.Include:
		return v.Meta
	case ast.Commodity:
		return v.Meta
	case ast.Open:
		return v.Meta
	case ast.Close:
		return v.Meta
	case ast.Transaction:
		return v.Meta
	case ast.Price:
		return v.Meta
	case ast.Balance:
		return v.Meta
	case ast.Pad:
		return v.Meta
	case ast.Note:
		return v.Meta
	case ast.Event:
		return v.Meta
	case ast.Custom:
		return v.Meta
	case ast.Document:
		return v.Meta
	case ast.Unknown:
		return v.Meta
	default:
		return ast.Meta{}
	}
}

// WriteFileAtomic writes data to path via temp file + rename.
// Creates parent directories as needed. Creates the file only when writing.
// The temp file is fsynced before rename so a crash after rename cannot leave
// an empty/partial journal that only lived in the page cache. After rename the
// parent directory is fsynced so the directory entry itself is durable; file
// Sync alone does not guarantee that the rename is on stable storage.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, ".contapila-ingest-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if remErr := os.Remove(tmpName); remErr != nil && !errors.Is(remErr, os.ErrNotExist) {
			// best-effort cleanup after successful rename or mid-write failure
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		closeErr := tmp.Close()
		return errors.Join(err, closeErr)
	}
	if err := tmp.Sync(); err != nil {
		closeErr := tmp.Close()
		return errors.Join(err, closeErr)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return syncDir(dir)
}

// syncDir fsyncs a directory so recent renames/creates are durable.
func syncDir(dir string) error {
	if dir == "" {
		dir = "."
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := d.Close(); closeErr != nil {
			// directory handle close is best-effort after Sync
		}
	}()
	return d.Sync()
}
