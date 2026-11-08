package dump

import (
	"errors"
	"testing"
)

func TestUnknownDialect(t *testing.T) {
	_, err := Extract("no-such-v1", "x", Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrUnknownDialect) {
		t.Fatalf("got %v want ErrUnknownDialect", err)
	}
}
