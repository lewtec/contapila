package filesys

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// ProviderRoot is the virtual absolute directory for a picked content tree.
// OpenProject walks parents with filepath, which cannot see above a provider.
const ProviderRoot = "/provider"

// FromIO adapts an io/fs.FS whose "." is the project tree.
// Callers pass absolute paths under root. root must be absolute.
func FromIO(fsys fs.FS, root string) FS {
	if fsys == nil {
		return OS{}
	}
	return ioFS{fsys: fsys, root: filepath.Clean(root)}
}

type ioFS struct {
	fsys fs.FS
	root string
}

func (a ioFS) rel(path string) (string, error) {
	path = filepath.Clean(path)
	if path == a.root {
		return ".", nil
	}
	rel, err := filepath.Rel(a.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fs.ErrNotExist
	}
	return filepath.ToSlash(rel), nil
}

func (a ioFS) ReadFile(path string) ([]byte, error) {
	rel, err := a.rel(path)
	if err != nil {
		return nil, err
	}
	return fs.ReadFile(a.fsys, rel)
}

func (a ioFS) Stat(path string) (fs.FileInfo, error) {
	rel, err := a.rel(path)
	if err != nil {
		return nil, err
	}
	return fs.Stat(a.fsys, rel)
}

func (a ioFS) ReadDir(path string) ([]fs.DirEntry, error) {
	rel, err := a.rel(path)
	if err != nil {
		return nil, err
	}
	return fs.ReadDir(a.fsys, rel)
}
