package queries_test

import (
	"testing"

	"github.com/lucasew/contapila-go/internal/plugins/queries"
	"github.com/lucasew/contapila-go/internal/web"
)

func TestQueriesPluginRegistered(t *testing.T) {
	web.BindPluginPages()
	found := false
	for _, id := range web.ReportPages() {
		if id == queries.PageID {
			found = true
		}
	}
	if !found {
		t.Fatal("queries page not in ReportPages after BindPluginPages")
	}
	if queries.PluginKey != "web_queries" {
		t.Fatalf("PluginKey=%q", queries.PluginKey)
	}
}
