package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNotDirectory is returned when -C/--directory names a file.
var ErrNotDirectory = errors.New("not a directory")

// DirArg is a project search start directory (CLI -C / --directory).
type DirArg struct {
	path string
}

func (d *DirArg) Parse(s string) error {
	if s == "" {
		d.path = ""
		return nil
	}
	abs, err := filepath.Abs(s)
	if err != nil {
		return fmt.Errorf("-C %s: %w", s, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("-C %s: %w", s, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("-C %s: %w", s, ErrNotDirectory)
	}
	d.path = abs
	return nil
}

func (d DirArg) Value() string { return d.path }
