// Package source holds Beancount (and other) text files with byte→line mapping.
package source

import (
	"github.com/lucasew/contapila-go/internal/filesys"
	"github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar"
)

// File is a named UTF-8 text buffer plus a line index for diagnostics and AST Meta.
// Build once per parse; Lines.LineAt is O(log lines). Offsets are byte offsets (tree-sitter).
type File struct {
	Path string
	// Text is the file contents (UTF-8). Prefer string over []byte so callers
	// stay in the encoding domain Go and ParseString already use.
	Text string
	// Lines maps byte offsets to 1-based lines (grammar.LineIndex).
	Lines *grammar.LineIndex
}

// New reads path from disk (as UTF-8 text) and builds the line index.
func New(path string) (*File, error) {
	return NewFS(filesys.OS{}, path)
}

// NewFS reads path via fsys and builds the line index.
func NewFS(fsys filesys.FS, path string) (*File, error) {
	if fsys == nil {
		fsys = filesys.OS{}
	}
	b, err := fsys.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return NewString(path, string(b)), nil
}

// NewString builds a File from an already-loaded string (tests, embeds, pipes).
func NewString(path, text string) *File {
	return &File{
		Path:  path,
		Text:  text,
		Lines: grammar.NewLineIndex(text),
	}
}

// Slice returns Text[start:end] safely (empty if out of range).
// start/end are source byte offsets, matching tree-sitter.
func (f *File) Slice(start, end int) string {
	if f == nil {
		return ""
	}
	if start < 0 || end > len(f.Text) || start > end {
		return ""
	}
	return f.Text[start:end]
}
