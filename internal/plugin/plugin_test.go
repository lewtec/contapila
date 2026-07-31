package plugin

import (
	"iter"
	"slices"
	"testing"

	"cuelang.org/go/cue/cuecontext"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/booking"
	"github.com/lucasew/contapila-go/internal/config"
)

type sampleSettings struct {
	MaxDays int `json:"max_days"`
}

type fakeWeb struct {
	id    string
	pages int
}

func (f *fakeWeb) ModuleID() string { return f.id }
func (f *fakeWeb) Page(page any)    { f.pages++ }

func TestRegisterTypedSetupAndProcess(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	var gotDays int
	RegisterTyped("sample", func(reg *Reg[sampleSettings]) {
		reg.Page("ignored-until-bound") // no-op when Web nil
		reg.OnProcess(func(s sampleSettings, h *Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
			gotDays = s.MaxDays
			dirs := slices.Collect(in)
			e := booking.New()
			e.Book(dirs)
			h.Engine = e
			return slices.Values(dirs)
		})
	})
	fw := &fakeWeb{}
	BindWeb(func(moduleID string) Web {
		fw.id = moduleID
		return fw
	})
	if fw.id != "sample" || fw.pages != 1 {
		t.Fatalf("BindWeb middleman id=%q pages=%d", fw.id, fw.pages)
	}
	if !config.IsKnownPlugin("sample") {
		t.Fatal("expected known plugin")
	}

	ctx := cuecontext.New()
	cfg := ctx.CompileString(`plugins: { sample: { enabled: true, settings: { max_days: 7 } } }`)
	if err := cfg.Err(); err != nil {
		t.Fatal(err)
	}
	h := &Host{}
	e, _, diags := RunStream(cfg, h, nil)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if e == nil {
		t.Fatal("expected engine from process")
	}
	if gotDays != 7 {
		t.Fatalf("gotDays=%d", gotDays)
	}
}

func TestDecodeBadSettingsType(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	RegisterTyped("sample", func(reg *Reg[sampleSettings]) {})
	ctx := cuecontext.New()
	cfg := ctx.CompileString(`plugins: { sample: { enabled: true, settings: { max_days: "nope" } } }`)
	if err := cfg.Err(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOptions(cfg); err == nil {
		t.Fatal("want decode error for bad max_days")
	}
}
