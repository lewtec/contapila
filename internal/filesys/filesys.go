// Package filesys is a small path-oriented file reader for project load and LSP overlays.
package filesys

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FS reads project files by absolute (or cleaned) path.
type FS interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (fs.FileInfo, error)
	ReadDir(path string) ([]fs.DirEntry, error)
}

// OS is the real filesystem.
type OS struct{}

func (OS) ReadFile(path string) ([]byte, error)  { return os.ReadFile(path) }
func (OS) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }
func (OS) ReadDir(path string) ([]fs.DirEntry, error) {
	return os.ReadDir(path)
}

// Overlay prefers in-memory text for ReadFile; Stat reports overlay size when set.
type Overlay struct {
	Base FS

	mu    sync.RWMutex
	files map[string]string // cleaned absolute path -> content
}

// NewOverlay wraps base (nil means OS).
func NewOverlay(base FS) *Overlay {
	if base == nil {
		base = OS{}
	}
	return &Overlay{Base: base, files: map[string]string{}}
}

func clean(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

// Set stores buffer text for path.
func (o *Overlay) Set(path, content string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.files[clean(path)] = content
}

// Delete removes an overlay entry.
func (o *Overlay) Delete(path string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.files, clean(path))
}

// Get returns overlay text if present.
func (o *Overlay) Get(path string) (string, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	s, ok := o.files[clean(path)]
	return s, ok
}

// Paths returns cleaned absolute overlay paths.
func (o *Overlay) Paths() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]string, 0, len(o.files))
	for p := range o.files {
		out = append(out, p)
	}
	return out
}

func (o *Overlay) ReadFile(path string) ([]byte, error) {
	p := clean(path)
	o.mu.RLock()
	if s, ok := o.files[p]; ok {
		o.mu.RUnlock()
		return []byte(s), nil
	}
	o.mu.RUnlock()
	return o.Base.ReadFile(p)
}

func (o *Overlay) Stat(path string) (fs.FileInfo, error) {
	p := clean(path)
	o.mu.RLock()
	if s, ok := o.files[p]; ok {
		o.mu.RUnlock()
		return overlayInfo{name: filepath.Base(p), size: int64(len(s)), mod: time.Now()}, nil
	}
	o.mu.RUnlock()
	return o.Base.Stat(p)
}

func (o *Overlay) ReadDir(path string) ([]fs.DirEntry, error) {
	return o.Base.ReadDir(clean(path))
}

type overlayInfo struct {
	name string
	size int64
	mod  time.Time
}

func (i overlayInfo) Name() string       { return i.name }
func (i overlayInfo) Size() int64        { return i.size }
func (i overlayInfo) Mode() fs.FileMode  { return 0o644 }
func (i overlayInfo) ModTime() time.Time { return i.mod }
func (i overlayInfo) IsDir() bool        { return false }
func (i overlayInfo) Sys() any           { return nil }
