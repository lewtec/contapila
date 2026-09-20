package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/lewtec/lewkit/x/cmd"
	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lucasew/contapila-go/internal/initfs"
)

type initCmd struct {
	Force cmd.Flag `long:"force" help:"write even if the directory is not empty"`
}

func (initCmd) Description() string {
	return `Copy a starter Project into the current directory

Writes contapila.cue plus personal and company ledgers (chart
and opening balances in each main.beancount). The directory
must be empty except for .git, unless --force is set.`
}

func (c *initCmd) Run(ctx context.Context) error {
	dir, err := projectCwd(ctx)
	if err != nil {
		return err
	}
	root, err := lewpath.Open(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if !c.Force.Value() {
		names, err := initfs.Occupants(root)
		if err != nil {
			return err
		}
		if len(names) > 0 {
			return fmt.Errorf("%w (pass --force to write anyway):\n%s", initfs.ErrNotEmpty, strings.Join(names, "\n"))
		}
	}
	src, err := initfs.Example()
	if err != nil {
		return err
	}
	if err := initfs.Copy(ctx, root, src); err != nil {
		return err
	}
	fmt.Printf("wrote starter project to %s\n", root.Name())
	return nil
}
