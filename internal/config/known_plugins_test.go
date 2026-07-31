package config

import "testing"

func TestKnownPlugins(t *testing.T) {
	resetKnownPluginsForTest()
	t.Cleanup(resetKnownPluginsForTest)

	if IsKnownPlugin("web_accounts") {
		t.Fatal("expected unknown before register")
	}
	RegisterKnownPlugin("web_accounts")
	RegisterKnownPlugin("web_accounts") // idempotent
	RegisterKnownPlugin("")
	if !IsKnownPlugin("web_accounts") {
		t.Fatal("expected known")
	}
	ids := KnownPluginIDs()
	if len(ids) != 1 || ids[0] != "web_accounts" {
		t.Fatalf("ids=%v", ids)
	}
}
