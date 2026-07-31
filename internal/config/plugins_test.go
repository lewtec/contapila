package config

import (
	"strings"
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
		ok, err := PluginEnabled(cfg.Value, "web_accounts")
		if err != nil || ok {
			t.Fatalf("missing key is off at CUE layer: ok=%v err=%v", ok, err)
		}
	})

	t.Run("disable accounts", func(t *testing.T) {
		user := `
plugins: {
	"web_accounts": { enabled: false }
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
		if flags["web_accounts"] {
			t.Fatal("accounts should be disabled")
		}
	})

	t.Run("enabled defaults false when object empty", func(t *testing.T) {
		user := `
plugins: {
	"web_future": {}
}
`
		cfg, err := Load([]byte(user), "t.cue", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		en := cfg.Value.LookupPath(cue.ParsePath(`plugins."web_future".enabled`))
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
		if flags["web_future"] {
			t.Fatal("empty object should decode as disabled")
		}
	})

	t.Run("explicit enable", func(t *testing.T) {
		user := `
plugins: {
	"web_future": { enabled: true }
}
`
		cfg, err := Load([]byte(user), "t.cue", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := PluginEnabled(cfg.Value, "web_future")
		if err != nil || !ok {
			t.Fatalf("want on: ok=%v err=%v", ok, err)
		}
	})
}

func TestPluginsEnabledFunc(t *testing.T) {
	cfg, err := Load([]byte(`plugins: { "web_accounts": { enabled: false } }`), "t.cue", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	fn, err := PluginsEnabledFunc(cfg.Value)
	if err != nil {
		t.Fatal(err)
	}
	if fn("web_accounts") {
		t.Fatal("want accounts off")
	}
	if fn("web/check") {
		t.Fatal("want missing keys off")
	}
}

func TestLoadWithPluginsFromJournal(t *testing.T) {
	cfg, err := LoadWithPlugins([]byte("{}"), "t.cue", nil, nil, []string{"web_accounts", "web_accounts", ""})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := PluginEnabled(cfg.Value, "web_accounts")
	if err != nil || !ok {
		t.Fatalf("journal plugin should enable: ok=%v err=%v", ok, err)
	}
	// User can still force off (unifies to false — conflict with true).
	_, err = LoadWithPlugins([]byte(`plugins: { "web_accounts": { enabled: false } }`), "t.cue", nil, nil, []string{"web_accounts"})
	if err == nil {
		t.Fatal("want unify conflict journal true vs cue false")
	}
}

func TestClosedConfigRejectsPluginTypos(t *testing.T) {
	// Top-level must be plugins (not plugin).
	_, err := Load([]byte(`
plugin: {
	check_closing: { enabled: true }
}
`), "t.cue", nil, nil)
	if err == nil {
		t.Fatal("want error for top-level plugin: (use plugins:)")
	}
	if !strings.Contains(err.Error(), "plugin") && !strings.Contains(err.Error(), "field not allowed") {
		// CUE wording varies; still require failure above.
		t.Logf("error was: %v", err)
	}

	// Field must be enabled (not enable).
	_, err = Load([]byte(`
plugins: {
	check_closing: { enable: true }
}
`), "t.cue", nil, nil)
	if err == nil {
		t.Fatal("want error for enable: (use enabled:)")
	}

	// Correct form still works.
	cfg, err := Load([]byte(`
plugins: {
	check_closing: { enabled: true }
}
`), "t.cue", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := PluginEnabled(cfg.Value, "check_closing")
	if err != nil || !ok {
		t.Fatalf("enabled: ok=%v err=%v", ok, err)
	}
}
