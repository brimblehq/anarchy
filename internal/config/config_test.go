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
	if fc.TeamID != "" || fc.TeamName != "" {
		t.Fatalf("team belongs to the logged-out account and should be cleared, got %+v", fc)
	}
	_ = os.Remove(path)
}

func TestTeamPrecedenceLinkOverConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("BRIMBLE_TEAM_ID", "")
	path, err := defaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveTeam(path, "file-team", "Acme"); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TeamID != "file-team" || cfg.TeamName != "Acme" || cfg.TeamSource != "config" {
		t.Fatalf("got team %q name %q source %q", cfg.TeamID, cfg.TeamName, cfg.TeamSource)
	}

	cfg, err = Load(Overrides{LinkTeamID: "link-team"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TeamID != "link-team" || cfg.TeamSource != "link" || cfg.TeamName != "" {
		t.Fatalf("got team %q name %q source %q", cfg.TeamID, cfg.TeamName, cfg.TeamSource)
	}

	if err := SaveTeam(path, "", "ignored"); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TeamID != "" || cfg.TeamSource != "" {
		t.Fatalf("expected personal workspace, got %q (%s)", cfg.TeamID, cfg.TeamSource)
	}
}

func TestLinkRoundTripFromSubdirectory(t *testing.T) {
	root := t.TempDir()
	if _, ok := mustFind(t, root); ok {
		t.Fatal("expected no link")
	}
	if _, err := SaveLink(root, Link{ProjectID: "p1", ProjectName: "web", TeamID: "t1"}); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "src", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	link, ok := mustFind(t, sub)
	if !ok || link.ProjectID != "p1" || link.ProjectName != "web" || link.TeamID != "t1" {
		t.Fatalf("got %+v ok=%v", link, ok)
	}
	if err := RemoveLink(root); err != nil {
		t.Fatal(err)
	}
	if _, ok := mustFind(t, sub); ok {
		t.Fatal("link should be removed")
	}
}

func mustFind(t *testing.T, dir string) (Link, bool) {
	t.Helper()
	link, _, ok, err := FindLink(dir)
	if err != nil {
		t.Fatal(err)
	}
	return link, ok
}
