// Package initfs embeds the example Project and copies it onto disk.
package initfs

import (
	"context"
	"errors"
	iofs "io/fs"
	"slices"

	lewfs "github.com/lewtec/lewkit/x/fs"
	lewpath "github.com/lewtec/lewkit/x/path"
	example "github.com/lucasew/contapila-go"
)

// ErrNotEmpty means dest has an entry other than .git.
var ErrNotEmpty = errors.New("directory is not empty")

// Example is the embedded starter Project (testdata/starter).
func Example() (iofs.FS, error) {
	return iofs.Sub(example.FS, lewpath.New("testdata", "starter").String())
}

// Occupants returns dest's top-level names, excluding .git only.
func Occupants(fsys iofs.FS) ([]string, error) {
	var names []string
	for p, err := range lewpath.New(".").IterDir(fsys) {
		if err != nil {
			return nil, err
		}
		if p == lewpath.New(".git") {
			continue
		}
		names = append(names, p.Name())
	}
	slices.Sort(names)
	return names, nil
}

// Copy writes src into dest using [lewfs.Copy]. Dest must already exist.
// Existing dest files are [iofs.ErrExist].
func Copy(ctx context.Context, dest lewfs.DestFS, src iofs.FS) error {
	return lewfs.Copy(ctx, dest, lewfs.Walk(ctx, src, nil))
}
