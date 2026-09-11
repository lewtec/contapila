package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lucasew/contapila-go/internal/dump"
	"github.com/lucasew/contapila-go/internal/dump/pdfdslipakv1"
	"github.com/lucasew/contapila-go/internal/dump/xlsxexcelizev1"
)

// dumpPassword is set from `dump --password` when the flag sits on the parent
// (before the dialect). Dialect commands also accept --password after the name.
var dumpPassword string

// ErrMissingDumpDialect is returned when `contapila dump` is run without a dialect subcommand.
var ErrMissingDumpDialect = errors.New("missing dialect subcommand (see contapila dump --help)")

type dumpCmd struct {
	cmdFlags
	Help     cmd.Flag      `short:"h" long:"help" help:"show help"`
	Password cmd.StringArg `short:"p" long:"password" help:"password for encrypted PDF or XLSX"`
	PDF      *dumpPDFCmd   `cmd:"pdf-dslipak-v1"`
	XLSX     *dumpXLSXCmd  `cmd:"xlsx-excelize-v1"`
}

func (dumpCmd) Description() string {
	return `Dump a source document as a versioned JSON element tree

Dump PDF or spreadsheet structure as compact JSON for stdlib-only extract scripts.

Each dialect is a subcommand ($format-$lib-v$n), also present in the JSON envelope.

Use --password for encrypted PDF/XLSX. The password is never written into the JSON.

Output is one compact JSON object on stdout:

  {"dialect":"…","source":"<path-as-given>","data":{"type":"…","children":[…]}}

Pipe into a language-stdlib script, then into contapila ingest as JSONL directives.`
}

func (c *dumpCmd) Run(context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	if c.Help.Value() {
		text, err := cmd.Usage[dumpCmd]("contapila dump")
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(os.Stdout, text)
		return err
	}
	return ErrMissingDumpDialect
}

type dumpPDFCmd struct {
	cmdFlags
	Password cmd.StringArg `short:"p" long:"password" help:"password for encrypted PDF or XLSX"`
	Path     cmd.StringArg
}

func (dumpPDFCmd) Description() string { return "Dump with dialect " + pdfdslipakv1.Dialect }

func (c *dumpPDFCmd) Run(context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	return runDump(pdfdslipakv1.Extract, c.Path.Value(), dumpPass(c.Password))
}

type dumpXLSXCmd struct {
	cmdFlags
	Password cmd.StringArg `short:"p" long:"password" help:"password for encrypted PDF or XLSX"`
	Path     cmd.StringArg
}

func (dumpXLSXCmd) Description() string { return "Dump with dialect " + xlsxexcelizev1.Dialect }

func (c *dumpXLSXCmd) Run(context.Context) error {
	if err := c.apply(); err != nil {
		return err
	}
	return runDump(xlsxexcelizev1.Extract, c.Path.Value(), dumpPass(c.Password))
}

func dumpPass(local cmd.StringArg) string {
	if v := local.Value(); v != "" {
		return v
	}
	return dumpPassword
}

func runDump(extract dump.Extractor, path, password string) error {
	if path == "" {
		return fmt.Errorf("%w: dump <dialect> <path>", cmd.ErrMissingValue)
	}
	data, err := extract(path, dump.Options{Password: password})
	if err != nil {
		return err
	}
	out, err := dump.MarshalCompact(data)
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	_, err = fmt.Fprintln(os.Stdout, string(out))
	return err
}
