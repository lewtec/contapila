package loader

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/contapila-go/internal/diag"
)

func TestLoadFileEmptyInclude(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main.beancount")
	if err := os.WriteFile(main, []byte("include \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, diags, err := LoadFile(t.Context(), main)
	if err == nil {
		t.Fatal("expected error for empty include")
	}
	if !errors.Is(err, ErrIncludePathEmpty) {
		t.Fatalf("err=%v want empty", err)
	}
	if !hasDiagMsg(diags, diag.Error, "include path is empty") {
		t.Fatalf("expected empty-path diagnostic, diags=%v", diags)
	}
}

func TestLoadFileIncludeDirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "main.beancount")
	if err := os.WriteFile(main, []byte("include \"subdir\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, diags, err := LoadFile(t.Context(), main)
	if err == nil {
		t.Fatal("expected error for directory include")
	}
	if !errors.Is(err, ErrIncludeIsDir) {
		t.Fatalf("err=%v want directory", err)
	}
	if !hasDiagMsg(diags, diag.Error, "include is a directory") {
		t.Fatalf("expected directory diagnostic, diags=%v", diags)
	}
}
