package config

import (
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
	APIKey string `json:"api_key,omitempty"`
	TeamID string `json:"team_id,omitempty"`
}

type Config struct {
	APIKey      string
	TeamID      string
	CoreBaseURL string
	AuthBaseURL string
	Output      string
	Timeout     time.Duration
	ConfigPath  string
}

type Overrides struct {
	APIKey      string
	TeamID      string
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
		APIKey:      firstNonEmpty(over.APIKey, os.Getenv("BRIMBLE_API_KEY"), file.APIKey),
		TeamID:      firstNonEmpty(over.TeamID, os.Getenv("BRIMBLE_TEAM_ID"), file.TeamID),
		CoreBaseURL: strings.TrimRight(firstNonEmpty(over.CoreBaseURL, os.Getenv("BRIMBLE_CORE_BASE_URL"), defaultCoreBaseURL), "/"),
		AuthBaseURL: strings.TrimRight(firstNonEmpty(over.AuthBaseURL, os.Getenv("BRIMBLE_AUTH_BASE_URL"), defaultAuthBaseURL), "/"),
		Output:      firstNonEmpty(over.Output, defaultOutput),
		Timeout:     defaultTimeout,
		ConfigPath:  path,
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
	if teamID != "" {
		existing.TeamID = strings.TrimSpace(teamID)
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

	existing.APIKey = ""
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
