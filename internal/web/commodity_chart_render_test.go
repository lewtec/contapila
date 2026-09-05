package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/engine"
)

func TestCommodityPageHasChart(t *testing.T) {
	_, f, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(f), "..", "..", "testdata", "example")
	p, pdb, _, err := engine.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/l/personal/commodity/USD", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		body := rr.Body.String()
		if len(body) > 300 {
			body = body[:300]
		}
		t.Fatalf("status %d body=%s", rr.Code, body)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "chart-commodity-price") {
		t.Fatalf("missing chart id; charts.js=%v kind=line=%v",
			strings.Contains(body, "charts.js"),
			strings.Contains(body, `"kind":"line"`))
	}
	if !strings.Contains(body, `"kind":"line"`) {
		t.Fatal("missing chart json payload")
	}
	if !strings.Contains(body, "charts.js") {
		t.Fatal("missing charts.js (NeedCharts false?)")
	}
}

func TestPricesPageHasChart(t *testing.T) {
	_, f, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(f), "..", "..", "testdata", "example")
	p, pdb, _, err := engine.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/l/personal/prices", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "chart-prices") {
		t.Fatal("expected prices page chart")
	}
	if !strings.Contains(body, `"kind":"line"`) {
		t.Fatal("expected line chart payload")
	}
}

func TestCommodityPageChartWithYearFilter(t *testing.T) {
	_, f, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(f), "..", "..", "testdata", "example")
	p, pdb, _, err := engine.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(p, pdb)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/l/personal/commodity/USD?time=year", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	body := rr.Body.String()
	t.Logf("status=%d hasChart=%v hasLine=%v", rr.Code, strings.Contains(body, "chart-commodity-price"), strings.Contains(body, `"kind":"line"`))
}
