package cmd

import (
	"context"
	"fmt"

	"github.com/brimbleapp/brimble-cli/internal/client"
	"github.com/brimbleapp/brimble-cli/internal/ui"
)

type ProjectRef struct {
	ID     string
	Name   string
	Status string
}

func (p ProjectRef) Display() string {
	if p.Name != "" {
		return p.Name
	}
	return p.ID
}

func (p ProjectRef) APIKey() string {
	if p.ID != "" {
		return p.ID
	}
	return p.Name
}

func pickProject(ctx context.Context, arg string) (ProjectRef, error) {
	if arg != "" {
		return resolveProject(ctx, arg)
	}
	if !ui.IsInteractive() {
		return ProjectRef{}, fmt.Errorf("project name is required")
	}

	projects, err := loadProjects(ctx)
	if err != nil {
		return ProjectRef{}, err
	}
	if len(projects) == 0 {
		return ProjectRef{}, fmt.Errorf("no projects found")
	}

	byName := map[string]ProjectRef{}
	opts := make([]ui.Option, 0, len(projects))
	for _, p := range projects {
		if p.Name == "" {
			continue
		}
		label := p.Name
		if p.Status != "" {
			label = label + "  [" + p.Status + "]"
		}
		opts = append(opts, ui.Option{Label: label, Value: p.Name})
		byName[p.Name] = p
	}
	if len(opts) == 0 {
		return ProjectRef{}, fmt.Errorf("no projects found")
	}

	name, err := ui.Select("Select a project", opts)
	if err != nil {
		return ProjectRef{}, err
	}
	return byName[name], nil
}

func resolveProject(ctx context.Context, arg string) (ProjectRef, error) {
	projects, err := loadProjects(ctx)
	if err != nil {
		return ProjectRef{ID: arg, Name: arg}, nil
	}
	for _, p := range projects {
		if p.Name == arg || p.ID == arg {
			return p, nil
		}
	}
	return ProjectRef{ID: arg, Name: arg}, nil
}

func loadProjects(ctx context.Context) ([]ProjectRef, error) {
	var data any
	err := ui.WithSpinner("Loading projects…", func() error {
		var err error
		data, err = coreClient.ListProjects(ctx, client.ListProjectsOpts{
			TeamID: cfg.TeamID,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return projectOptions(unwrapList(data)), nil
}

func pickTeamName(ctx context.Context, arg string) (string, error) {
	if arg != "" {
		return arg, nil
	}
	if !ui.IsInteractive() {
		return "", fmt.Errorf("team name is required")
	}

	var data any
	err := ui.WithSpinner("Loading teams…", func() error {
		var err error
		data, err = coreClient.ListTeams(ctx)
		return err
	})
	if err != nil {
		return "", err
	}

	teams := teamOptions(unwrapList(data))
	if len(teams) == 0 {
		return "", fmt.Errorf("no teams found")
	}

	opts := make([]ui.Option, 0, len(teams))
	for _, t := range teams {
		if t.Name == "" {
			continue
		}
		opts = append(opts, ui.Option{Label: t.Name, Value: t.Name})
	}
	if len(opts) == 0 {
		return "", fmt.Errorf("no teams found")
	}
	return ui.Select("Select a team", opts)
}
