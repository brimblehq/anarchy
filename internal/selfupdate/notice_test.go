package selfupdate

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRefreshCachesLatest(t *testing.T) {
	releaseServer(t, "v0.4.0", nil, false)
	path := filepath.Join(t.TempDir(), "brimble", "update-check.json")
	if s := LoadState(path); !s.CheckedAt.IsZero() || s.Latest != "" {
		t.Fatalf("missing file should load empty, got %+v", s)
	}
	if _, err := Refresh(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	s := LoadState(path)
	if s.Latest != "v0.4.0" || s.CheckedAt.IsZero() {
		t.Fatalf("state = %+v", s)
	}
}

func TestRefreshWithoutReleasesIsCached(t *testing.T) {
	releaseServer(t, "", nil, false)
	path := filepath.Join(t.TempDir(), "update-check.json")
	if _, err := Refresh(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if s := LoadState(path); s.CheckedAt.IsZero() || s.Latest != "" {
		t.Fatalf("state = %+v", s)
	}
}

func TestIsSourceVersion(t *testing.T) {
	for v, want := range map[string]bool{"dev": true, "": true, "v0.1.0-1-g43d04a5-dirty": true, "0.1.0": false, "v1.2.3": false} {
		if got := IsSourceVersion(v); got != want {
			t.Errorf("IsSourceVersion(%q) = %v, want %v", v, got, want)
		}
	}
}
