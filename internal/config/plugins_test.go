package config

import (
	"testing"

	"cuelang.org/go/cue"
)

func TestPluginFlags(t *testing.T) {
	t.Run("absent plugins", func(t *testing.T) {
		cfg, err := Load([]byte("{}"), "t.cue", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		flags, err := PluginFlags(cfg.Value)
		if err != nil {
			t.Fatal(err)
		}
		if flags != nil {
			t.Fatalf("want nil flags, got %v", flags)
		}
		ok, err := PluginEnabled(cfg.Value, "web/accounts")
		if err != nil || !ok {
			t.Fatalf("default enabled: ok=%v err=%v", ok, err)
		}
	})

	t.Run("disable one plugin", func(t *testing.T) {
		user := `
plugins: {
	"web/accounts": { enabled: false }
	"web/check": { enabled: true }
}
`
		cfg, err := Load([]byte(user), "t.cue", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		flags, err := PluginFlags(cfg.Value)
		if err != nil {
			t.Fatal(err)
		}
		if flags["web/accounts"] {
			t.Fatal("accounts should be disabled")
		}
		if !flags["web/check"] {
			t.Fatal("check should be enabled")
		}
		// Unknown key stays on.
		ok, err := PluginEnabled(cfg.Value, "web/journal")
		if err != nil || !ok {
			t.Fatalf("missing key defaults on: %v %v", ok, err)
		}
	})

	t.Run("enabled defaults true when object empty", func(t *testing.T) {
		user := `
plugins: {
	"web/accounts": {}
}
`
		cfg, err := Load([]byte(user), "t.cue", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		// *true default from prelude #Plugin
		en := cfg.Value.LookupPath(cue.ParsePath(`plugins."web/accounts".enabled`))
		b, err := en.Bool()
		if err != nil {
			t.Fatal(err)
		}
		if !b {
			t.Fatal("expected default enabled true")
		}
	})
}

func TestPluginsEnabledFunc(t *testing.T) {
	cfg, err := Load([]byte(`plugins: { "web/accounts": { enabled: false } }`), "t.cue", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	fn, err := PluginsEnabledFunc(cfg.Value)
	if err != nil {
		t.Fatal(err)
	}
	if fn("web/accounts") {
		t.Fatal("want accounts off")
	}
	if !fn("web/check") {
		t.Fatal("want check on")
	}
}
