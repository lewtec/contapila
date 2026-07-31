package plugin

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/diag"
)

type sampleSettings struct {
	MaxDays int `json:"max_days"`
}

func TestRegisterTypedDecode(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	var gotDays int
	RegisterTyped(TypedModule[sampleSettings]{
		ID: "sample",
		Book: func(ctx BookContext, dirs []ast.Directive, opts Options[sampleSettings]) (*booking.Engine, []ast.Directive, diag.List) {
			gotDays = opts.Settings.MaxDays
			e := booking.New()
			e.Book(dirs)
			return e, dirs, e.Diags
		},
	})
	if !config.IsKnownPlugin("sample") {
		t.Fatal("expected known plugin")
	}

	ctx := cuecontext.New()
	cfg := ctx.CompileString(`plugins: { sample: { enabled: true, settings: { max_days: 7 } } }`)
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
	o, ok := opts.(Options[sampleSettings])
	if !ok || !o.Enabled || o.Settings.MaxDays != 7 {
		t.Fatalf("opts=%+v", opts)
	}
	_, _, _ = mods[0].Book(BookContext{}, nil, opts)
	if gotDays != 7 {
		t.Fatalf("gotDays=%d", gotDays)
	}
}

func TestDecodeBadSettingsType(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	RegisterTyped(TypedModule[sampleSettings]{ID: "sample"})
	ctx := cuecontext.New()
	cfg := ctx.CompileString(`plugins: { sample: { enabled: true, settings: { max_days: "nope" } } }`)
	if err := cfg.Err(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOptions(cfg); err == nil {
		t.Fatal("want decode error for bad max_days")
	}
}
