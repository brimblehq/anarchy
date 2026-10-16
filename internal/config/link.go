package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	linkDirName  = ".brimble"
	linkFileName = "project.json"
)

// Link ties a local directory to a Brimble project, so commands run inside it
// don't need a project argument.
type Link struct {
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
	TeamID      string `json:"teamId,omitempty"`
}

// FindLink walks up from dir looking for .brimble/project.json. It returns the
// link and the directory that contains it, or ok=false when none exists.
func FindLink(dir string) (link Link, root string, ok bool, err error) {
	dir, err = filepath.Abs(dir)
	if err != nil {
		return Link{}, "", false, err
	}
	for {
		path := filepath.Join(dir, linkDirName, linkFileName)
		b, err := os.ReadFile(path)
		if err == nil {
			if err := json.Unmarshal(b, &link); err != nil {
				return Link{}, "", false, fmt.Errorf("parse %s: %w", path, err)
			}
			if link.ProjectID == "" {
				return Link{}, "", false, fmt.Errorf("%s has no projectId — run brimble link again", path)
			}
			return link, dir, true, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return Link{}, "", false, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Link{}, "", false, nil
		}
		dir = parent
	}
}

func SaveLink(dir string, link Link) (string, error) {
	path := filepath.Join(dir, linkDirName, linkFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(link, "", "  ")
	if err != nil {
		return "", err
	}
	b = append(b, '\n')
	return path, os.WriteFile(path, b, 0o644)
}

// RemoveLink deletes the link file in dir and the .brimble folder when it is empty.
func RemoveLink(dir string) error {
	path := filepath.Join(dir, linkDirName, linkFileName)
	if err := os.Remove(path); err != nil {
		return err
	}
	_ = os.Remove(filepath.Dir(path))
	return nil
}
