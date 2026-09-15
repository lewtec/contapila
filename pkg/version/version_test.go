package version

import (
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })

	tests := []struct {
		name string
		set  string
		want string
	}{
		{name: "default dev", set: "dev", want: "dev"},
		{name: "empty falls back to dev", set: "", want: "dev"},
		{name: "whitespace only falls back to dev", set: "   ", want: "dev"},
		{name: "non-empty passthrough", set: "1.2.3", want: "1.2.3"},
		{name: "trims surrounding space", set: "  v0.4.0  ", want: "v0.4.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version = tt.set
			if got := Version(); got != tt.want {
				t.Errorf("Version() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestBuildID(t *testing.T) {
	id := BuildID()
	if id == "" {
		t.Fatal("BuildID() returned empty string")
	}
	// In a test binary: short VCS revision, "dev" (no vcs setting), or "unknown".
	if id != "dev" && id != "unknown" && len(id) > 8 {
		t.Errorf("BuildID() = %q; expected dev, unknown, or at most 8-char revision", id)
	}
	// Stable across calls in the same process.
	if again := BuildID(); again != id {
		t.Errorf("BuildID() not stable: first %q, second %q", id, again)
	}
}

func TestGetBuildID(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })

	version = "1.0.0"
	got := GetBuildID()
	wantPrefix := Version() + "-"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("GetBuildID() = %q; want prefix %q", got, wantPrefix)
	}
	suffix := strings.TrimPrefix(got, wantPrefix)
	if suffix == "" {
		t.Fatalf("GetBuildID() = %q; missing BuildID suffix", got)
	}
	if suffix != BuildID() {
		t.Errorf("GetBuildID() suffix %q != BuildID() %q", suffix, BuildID())
	}
	if !strings.Contains(got, Version()) || !strings.Contains(got, BuildID()) {
		t.Errorf("GetBuildID() = %q; want to contain Version()=%q and BuildID()=%q",
			got, Version(), BuildID())
	}
}
