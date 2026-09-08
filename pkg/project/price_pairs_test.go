package project_test

import (
	"path/filepath"
	"testing"

	"cuelang.org/go/cue"
	"github.com/lucasew/contapila-go/pkg/project"
)

func TestExamplePricePairsInjected(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "example")
	p, err := project.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	base := p.Config.Value.LookupPath(cue.ParsePath(`price_pairs."USD|BRL".base`))
	s, err := base.String()
	if err != nil || s != "USD" {
		t.Fatalf("base=%q err=%v", s, err)
	}
	it, err := p.Config.Value.LookupPath(cue.ParsePath("price_pairs")).Fields()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for it.Next() {
		n++
	}
	if n < 5 {
		t.Fatalf("pair count %d", n)
	}
}
