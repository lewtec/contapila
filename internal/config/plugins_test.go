package config

import (
	"testing"

	"cuelang.org/go/cue"
)

func TestPluginFlags(t *testing.T) {
	t.Run("empty config has no plugin flags", func(t *testing.T) {
		cfg, err := Load([]byte("{}"), "t.cue", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		flags, err := PluginFlags(cfg.Value)
		if err != nil {
			t.Fatal(err)
		}
		// Open plugins: [string]: #Plugin with no concrete keys → empty/nil.
		if len(flags) != 0 {
			t.Fatalf("want no concrete flags, got %v", flags)
		}
		ok, err := PluginEnabled(cfg.Value, "web/accounts")
		if err != nil || ok {
			t.Fatalf("missing key is off at CUE layer: ok=%v err=%v", ok, err)
		}
	})

	t.Run("disable accounts", func(t *testing.T) {
		user := `
plugins: {
	"web/accounts": { enabled: false }
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
	})

	t.Run("enabled defaults false when object empty", func(t *testing.T) {
		user := `
plugins: {
	"web/future": {}
}
`
		cfg, err := Load([]byte(user), "t.cue", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		en := cfg.Value.LookupPath(cue.ParsePath(`plugins."web/future".enabled`))
		b, err := en.Bool()
		if err != nil {
			t.Fatal(err)
		}
		if b {
			t.Fatal("expected default enabled false")
		}
		flags, err := PluginFlags(cfg.Value)
		if err != nil {
			t.Fatal(err)
		}
		if flags["web/future"] {
			t.Fatal("empty object should decode as disabled")
		}
	})

	t.Run("explicit enable", func(t *testing.T) {
		user := `
plugins: {
	"web/future": { enabled: true }
}
`
		cfg, err := Load([]byte(user), "t.cue", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := PluginEnabled(cfg.Value, "web/future")
		if err != nil || !ok {
			t.Fatalf("want on: ok=%v err=%v", ok, err)
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
	if fn("web/check") {
		t.Fatal("want missing keys off")
	}
}
