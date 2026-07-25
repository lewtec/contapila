package pdfdslipakv1

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lucasew/contapila-go/internal/dump"
)

func TestExtractSample(t *testing.T) {
	path := filepath.Join("testdata", "sample.pdf")
	got, err := Extract(path, dump.Options{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	got.Source = "sample.pdf"

	wantBytes, err := os.ReadFile(filepath.Join("testdata", "sample.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var want dump.ExtractedData
	if err := json.Unmarshal(wantBytes, &want); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}

	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("dump mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestDialectRegistered(t *testing.T) {
	if _, ok := dump.Lookup(Dialect); !ok {
		t.Fatalf("dialect %q not registered", Dialect)
	}
}

// TestExtractClosesFile ensures Extract does not leave the source PDF open.
// Before the fix, openPDF leaked the os.File (pdf.Reader has no Close).
func TestExtractClosesFile(t *testing.T) {
	src := filepath.Join("testdata", "sample.pdf")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Extract(path, dump.Options{}); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// On Linux, unlinking succeeds while the file is still open — check /proc
	// for an fd still pointing at the path.
	if runtime.GOOS == "linux" {
		if fdStillOpen(t, abs) {
			t.Fatalf("source PDF still open after Extract: %s", abs)
		}
	}

	// Second extract must still work (no stuck handle preventing reopen).
	if _, err := Extract(path, dump.Options{}); err != nil {
		t.Fatalf("second Extract: %v", err)
	}
	if runtime.GOOS == "linux" && fdStillOpen(t, abs) {
		t.Fatalf("source PDF still open after second Extract: %s", abs)
	}
}

func fdStillOpen(t *testing.T, absPath string) bool {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skipf("cannot read /proc/self/fd: %v", err)
	}
	want, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		want = absPath
	}
	for _, e := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", e.Name()))
		if err != nil {
			continue
		}
		// Readlink may return the path as opened (not always eval'd).
		if target == absPath || target == want {
			return true
		}
		if eval, err := filepath.EvalSymlinks(target); err == nil && eval == want {
			return true
		}
		// Kernel may append " (deleted)" after unlink; still a leak signal.
		if strings.HasPrefix(target, absPath) || strings.HasPrefix(target, want) {
			return true
		}
	}
	return false
}
