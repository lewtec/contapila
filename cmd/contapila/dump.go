package main

import (
	"fmt"

	"github.com/lucasew/contapila-go/internal/dump"
	"github.com/lucasew/contapila-go/internal/dump/pdfdslipakv1"
	"github.com/lucasew/contapila-go/internal/dump/xlsxexcelizev1"
	"github.com/spf13/cobra"
)

func dumpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump",
		Short: "Dump a source document as a versioned JSON element tree",
		Long: `Dump PDF or spreadsheet structure as compact JSON for stdlib-only extract scripts.

Each dialect is a subcommand ($format-$lib-v$n), also present in the JSON envelope.

Output is one compact JSON object on stdout:

  {"dialect":"…","source":"<path-as-given>","data":{"type":"…","children":[…]}}

Pipe into a language-stdlib script, then into contapila ingest as JSONL directives.`,
		// Parent is not a leaf: require a dialect subcommand (Cobra resolves those first).
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("missing dialect subcommand (see contapila dump --help)")
		},
	}
	cmd.AddCommand(
		dumpDialectCmd(pdfdslipakv1.Dialect, pdfdslipakv1.Extract),
		dumpDialectCmd(xlsxexcelizev1.Dialect, xlsxexcelizev1.Extract),
	)
	return cmd
}

func dumpDialectCmd(dialect string, extract dump.Extractor) *cobra.Command {
	return &cobra.Command{
		Use:   dialect + " <path>",
		Short: "Dump with dialect " + dialect,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := extract(args[0])
			if err != nil {
				return err
			}
			out, err := dump.MarshalCompact(data)
			if err != nil {
				return fmt.Errorf("marshal json: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return err
		},
	}
}
