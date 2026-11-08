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
	id       string
	pages    int
	palettes int
}

func (f *fakeWeb) ModuleID() string { return f.id }
func (f *fakeWeb) Page(page any)    { f.pages++ }
func (f *fakeWeb) Palette(p any)    { f.palettes++ }

func TestRegisterTypedSetupAndProcess(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	var gotDays int
	RegisterTyped("sample", func(reg *Reg[sampleSettings]) {
		reg.Page("ignored-until-bound") // no-op when Web nil
		reg.Palette("ignored-until-bound")
		reg.OnProcess(PhaseBook, func(s sampleSettings, h *Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
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
	if fw.id != "sample" || fw.pages != 1 || fw.palettes != 1 {
		t.Fatalf("BindWeb middleman id=%q pages=%d palettes=%d", fw.id, fw.pages, fw.palettes)
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

func TestDefaultOnWhenKeyOmitted(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	ran := false
	RegisterTyped("core_expand", func(reg *Reg[struct{}]) {
		reg.DefaultOn()
		reg.OnProcess(PhaseEarly, func(_ struct{}, h *Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
			ran = true
			return in
		})
	})
	ctx := cuecontext.New()
	cfg := ctx.CompileString(`plugins: {}`)
	if err := cfg.Err(); err != nil {
		t.Fatal(err)
	}
	_, _, _ = RunPhase(cfg, &Host{}, nil, PhaseEarly)
	if !ran {
		t.Fatal("DefaultOn module should run when key omitted")
	}
	if !IsEnabled(cfg, "core_expand") {
		t.Fatal("IsEnabled should be true for DefaultOn omitted key")
	}
}

func TestDefaultOnExplicitFalse(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	ran := false
	RegisterTyped("core_expand", func(reg *Reg[struct{}]) {
		reg.DefaultOn()
		reg.OnProcess(PhaseEarly, func(_ struct{}, h *Host, in iter.Seq[ast.Directive]) iter.Seq[ast.Directive] {
			ran = true
			return in
		})
	})
	ctx := cuecontext.New()
	cfg := ctx.CompileString(`plugins: { core_expand: { enabled: false } }`)
	if err := cfg.Err(); err != nil {
		t.Fatal(err)
	}
	_, _, _ = RunPhase(cfg, &Host{}, nil, PhaseEarly)
	if ran {
		t.Fatal("explicit enabled:false must disable DefaultOn module")
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
