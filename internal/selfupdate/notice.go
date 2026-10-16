package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// CheckInterval is how often the update notice asks GitHub for a new release.
const CheckInterval = 24 * time.Hour

// State is the cached result of the last update check.
type State struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

// StatePath is where the update check is cached, next to the CLI config.
func StatePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "brimble", "update-check.json"), nil
}

func LoadState(path string) State {
	var s State
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// Refresh asks GitHub for the latest release and caches it. A repository
// without releases is cached too, so it isn't asked again until the next interval.
func Refresh(ctx context.Context, path string) (State, error) {
	latest, err := Latest(ctx)
	if err != nil && !errors.Is(err, ErrNoRelease) {
		return State{}, err
	}
	s := State{CheckedAt: time.Now(), Latest: latest}
	b, _ := json.Marshal(s)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return s, err
	}
	return s, os.WriteFile(path, b, 0o600)
}

// IsSourceVersion reports builds from a checkout ("dev" or git describe
// output), which can't be upgraded to a release.
func IsSourceVersion(version string) bool {
	return version == "" || version == "dev" || gitDescribe.MatchString(version)
}
