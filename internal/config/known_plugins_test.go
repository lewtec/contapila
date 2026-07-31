package config

import "testing"

func TestKnownPlugins(t *testing.T) {
	resetKnownPluginsForTest()
	t.Cleanup(resetKnownPluginsForTest)

	if IsKnownPlugin("web/accounts") {
		t.Fatal("expected unknown before register")
	}
	RegisterKnownPlugin("web/accounts")
	RegisterKnownPlugin("web/accounts") // idempotent
	RegisterKnownPlugin("")
	if !IsKnownPlugin("web/accounts") {
		t.Fatal("expected known")
	}
	ids := KnownPluginIDs()
	if len(ids) != 1 || ids[0] != "web/accounts" {
		t.Fatalf("ids=%v", ids)
	}
}
