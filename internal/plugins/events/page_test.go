package events_test

import (
	"testing"

	"github.com/lucasew/contapila-go/internal/plugins/events"
	"github.com/lucasew/contapila-go/internal/web"
)

func TestEventsPluginRegistered(t *testing.T) {
	web.BindPluginPages()
	found := false
	for _, id := range web.ReportPages() {
		if id == events.PageID {
			found = true
		}
	}
	if !found {
		t.Fatal("events page not in ReportPages after BindPluginPages")
	}
	if events.PluginKey != "web/events" {
		t.Fatalf("PluginKey=%q", events.PluginKey)
	}
}
