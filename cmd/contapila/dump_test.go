package main

import (
	"path/filepath"
	"testing"

	"github.com/lewtec/lewkit/x/cmd"
	lewtest "github.com/lewtec/lewkit/x/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDumpUnknownDialect(t *testing.T) {
	err := cmd.ParseErr[cmd.App[root]](t, "dump", "nope-v1", "x.pdf")
	require.ErrorIs(t, err, cmd.ErrUnknownCommand)
}

func TestDumpMissingDialect(t *testing.T) {
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "dump")
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, "pdf-dslipak-v1")
	assert.Contains(t, out, "xlsx-excelize-v1")
}

func TestDumpPDFFixture(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "dump", "pdfdslipakv1", "testdata", "sample.pdf")
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "dump", "pdf-dslipak-v1", path)
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, `"dialect":"pdf-dslipak-v1"`)
	assert.Contains(t, out, `"type":"document"`)
}

func TestDumpXLSXFixture(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "dump", "xlsxexcelizev1", "testdata", "sample.xlsx")
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "dump", "xlsx-excelize-v1", path)
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, `"dialect":"xlsx-excelize-v1"`)
	assert.Contains(t, out, `"type":"workbook"`)
}

func TestDumpPasswordFlagAccepted(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "dump", "pdfdslipakv1", "testdata", "sample.pdf")
	lewtest.DiscardSlog(t)
	app := cmd.ParseOK[cmd.App[root]](t, "dump", "--password", "unused", "pdf-dslipak-v1", path)
	out := lewtest.Stdout(t, func() {
		require.NoError(t, app.Run(t.Context()))
	})
	assert.Contains(t, out, `"dialect":"pdf-dslipak-v1"`)
}
