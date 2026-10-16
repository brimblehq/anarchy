package config

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultCoreBaseURL = "https://api.brimble.io/core"
	defaultAuthBaseURL = "https://api.brimble.io/auth"
	defaultTimeout     = 30 * time.Second
	defaultOutput      = "table"
)

type FileConfig struct {
	APIKey   string `json:"api_key,omitempty"`
	TeamID   string `json:"team_id,omitempty"`
	TeamName string `json:"team_name,omitempty"`
}

type Config struct {
	APIKey      string
	TeamID      string
	TeamName    string // display name of TeamID, when known
	TeamSource  string // flag, env, link, config or "" (personal workspace)
	CoreBaseURL string
	AuthBaseURL string
	Output      string
	Timeout     time.Duration
	ConfigPath  string
}

type Overrides struct {
	APIKey      string
	TeamID      string
	LinkTeamID  string // team of the project linked to the working directory
	CoreBaseURL string
	AuthBaseURL string
	Output      string
	Timeout     time.Duration
}

func Load(over Overrides) (Config, error) {
	path, err := defaultConfigPath()
	if err != nil {
		return Config{}, err
	}

	file, _ := readFile(path)

	cfg := Config{
		APIKey:      cmp.Or(over.APIKey, os.Getenv("BRIMBLE_API_KEY"), file.APIKey),
		CoreBaseURL: strings.TrimRight(strings.TrimSpace(cmp.Or(over.CoreBaseURL, os.Getenv("BRIMBLE_CORE_BASE_URL"), defaultCoreBaseURL)), "/"),
		AuthBaseURL: strings.TrimRight(strings.TrimSpace(cmp.Or(over.AuthBaseURL, os.Getenv("BRIMBLE_AUTH_BASE_URL"), defaultAuthBaseURL)), "/"),
		Output:      cmp.Or(over.Output, defaultOutput),
		Timeout:     defaultTimeout,
		ConfigPath:  path,
	}

	switch {
	case strings.TrimSpace(over.TeamID) != "":
		cfg.TeamID, cfg.TeamSource = over.TeamID, "flag"
	case strings.TrimSpace(os.Getenv("BRIMBLE_TEAM_ID")) != "":
		cfg.TeamID, cfg.TeamSource = os.Getenv("BRIMBLE_TEAM_ID"), "env"
	case strings.TrimSpace(over.LinkTeamID) != "":
		cfg.TeamID, cfg.TeamSource = over.LinkTeamID, "link"
	case file.TeamID != "":
		cfg.TeamID, cfg.TeamSource, cfg.TeamName = file.TeamID, "config", file.TeamName
	}
	if cfg.TeamID == file.TeamID {
		cfg.TeamName = file.TeamName
	}

	if over.Timeout > 0 {
		cfg.Timeout = over.Timeout
	}

	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.TeamID = strings.TrimSpace(cfg.TeamID)

	if cfg.Output != "table" && cfg.Output != "json" {
		return Config{}, fmt.Errorf("output must be table or json, got %q", cfg.Output)
	}

	return cfg, nil
}

func SaveCredentials(path, apiKey, teamID string) error {
	if path == "" {
		var err error
		path, err = defaultConfigPath()
		if err != nil {
			return err
		}
	}

	existing, _ := readFile(path)
	if apiKey != "" {
		existing.APIKey = strings.TrimSpace(apiKey)
	}
	if teamID != "" && teamID != existing.TeamID {
		existing.TeamID = strings.TrimSpace(teamID)
		existing.TeamName = ""
	}

	return writeFile(path, existing)
}

func ClearCredentials(path string) error {
	if path == "" {
		var err error
		path, err = defaultConfigPath()
		if err != nil {
			return err
		}
	}

	existing, err := readFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// The default team belongs to the account being logged out.
	existing.APIKey = ""
	existing.TeamID = ""
	existing.TeamName = ""
	return writeFile(path, existing)
}

// SaveTeam sets the default team. An empty teamID switches back to the
// personal workspace.
func SaveTeam(path, teamID, teamName string) error {
	if path == "" {
		var err error
		path, err = defaultConfigPath()
		if err != nil {
			return err
		}
	}
	existing, err := readFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	existing.TeamID = strings.TrimSpace(teamID)
	existing.TeamName = strings.TrimSpace(teamName)
	if existing.TeamID == "" {
		existing.TeamName = ""
	}
	return writeFile(path, existing)
}

func defaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(dir, "brimble", "config.json"), nil
}

func readFile(path string) (FileConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return FileConfig{}, err
	}
	var fc FileConfig
	if err := json.Unmarshal(b, &fc); err != nil {
		return FileConfig{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return fc, nil
}

func writeFile(path string, fc FileConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	b, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}
