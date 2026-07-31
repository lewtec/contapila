package config

import (
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

func TestValidatePluginID(t *testing.T) {
	if err := ValidatePluginID("web_accounts"); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePluginID(""); err == nil {
		t.Fatal("want empty error")
	}
	err := ValidatePluginID("web/accounts")
	if err == nil || !strings.Contains(err.Error(), "/") {
		t.Fatalf("want slash error, got %v", err)
	}
}

func TestValidatePluginMapKeysRejectsSlash(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`plugins: { "web/accounts": { enabled: true } }`)
	if err := v.Err(); err != nil {
		t.Fatal(err)
	}
	err := ValidatePluginMapKeys(v)
	if err == nil || !strings.Contains(err.Error(), "/") {
		t.Fatalf("want slash error, got %v", err)
	}
}

func TestLoadRejectsSlashPluginKey(t *testing.T) {
	_, err := Load([]byte(`plugins: { "web/accounts": { enabled: true } }`), "t.cue", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "/") {
		t.Fatalf("want load error for slash key, got %v", err)
	}
}

func TestRegisterKnownPluginPanicsOnSlash(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("want panic")
		}
		if s, ok := r.(string); !ok || !strings.Contains(s, "/") {
			t.Fatalf("panic=%v", r)
		}
	}()
	RegisterKnownPlugin("web/bad")
}
