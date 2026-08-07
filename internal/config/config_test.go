package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPrecedenceFlagOverEnvOverFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := writeFile(path, FileConfig{APIKey: "file-key", TeamID: "file-team"}); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BRIMBLE_API_KEY", "env-key")
	t.Setenv("BRIMBLE_TEAM_ID", "env-team")
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := Load(Overrides{
		APIKey: "flag-key",
		TeamID: "flag-team",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "flag-key" {
		t.Fatalf("api key = %q, want flag-key", cfg.APIKey)
	}
	if cfg.TeamID != "flag-team" {
		t.Fatalf("team = %q, want flag-team", cfg.TeamID)
	}
}

func TestLoadEnvOverEmptyFlag(t *testing.T) {
	t.Setenv("BRIMBLE_API_KEY", "env-key")
	t.Setenv("BRIMBLE_TEAM_ID", "env-team")
	t.Setenv("BRIMBLE_CORE_BASE_URL", "https://core.example")
	t.Setenv("BRIMBLE_AUTH_BASE_URL", "https://auth.example")

	cfg, err := Load(Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "env-key" {
		t.Fatalf("api key = %q", cfg.APIKey)
	}
	if cfg.CoreBaseURL != "https://core.example" {
		t.Fatalf("core url = %q", cfg.CoreBaseURL)
	}
	if cfg.AuthBaseURL != "https://auth.example" {
		t.Fatalf("auth url = %q", cfg.AuthBaseURL)
	}
}

func TestSaveAndClearCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brimble", "config.json")
	if err := SaveCredentials(path, "br_test", "team1"); err != nil {
		t.Fatal(err)
	}
	fc, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if fc.APIKey != "br_test" || fc.TeamID != "team1" {
		t.Fatalf("got %+v", fc)
	}
	if err := ClearCredentials(path); err != nil {
		t.Fatal(err)
	}
	fc, err = readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if fc.APIKey != "" {
		t.Fatalf("api key should be cleared, got %q", fc.APIKey)
	}
	if fc.TeamID != "team1" {
		t.Fatalf("team should remain, got %q", fc.TeamID)
	}
	_ = os.Remove(path)
}
