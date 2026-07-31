package plugin

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/diag"
)

type sampleOpts struct {
	Enabled bool `json:"enabled"`
	MaxDays int  `json:"max_days"`
}

func TestRegisterTypedDecode(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	var gotDays int
	RegisterTyped(TypedModule[sampleOpts]{
		ID: "sample",
		Book: func(ctx BookContext, dirs []ast.Directive, opts sampleOpts) (*booking.Engine, []ast.Directive, diag.List) {
			gotDays = opts.MaxDays
			e := booking.New()
			e.Book(dirs)
			return e, dirs, e.Diags
		},
	})
	if !config.IsKnownPlugin("sample") {
		t.Fatal("expected known plugin")
	}

	ctx := cuecontext.New()
	cfg := ctx.CompileString(`plugins: { sample: { enabled: true, max_days: 7 } }`)
	if err := cfg.Err(); err != nil {
		t.Fatal(err)
	}
	mods := EnabledBookModules(cfg)
	if len(mods) != 1 {
		t.Fatalf("book mods: %d", len(mods))
	}
	opts, err := DecodeOptions(cfg, mods[0])
	if err != nil {
		t.Fatal(err)
	}
	o, ok := opts.(sampleOpts)
	if !ok || o.MaxDays != 7 || !o.Enabled {
		t.Fatalf("opts=%+v", opts)
	}
	_, _, _ = mods[0].Book(BookContext{}, nil, opts)
	if gotDays != 7 {
		t.Fatalf("gotDays=%d", gotDays)
	}
}

func TestDecodeBadType(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	RegisterTyped(TypedModule[sampleOpts]{ID: "sample"})
	ctx := cuecontext.New()
	cfg := ctx.CompileString(`plugins: { sample: { enabled: true, max_days: "nope" } }`)
	if err := cfg.Err(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOptions(cfg); err == nil {
		t.Fatal("want decode error for bad max_days")
	}
}
