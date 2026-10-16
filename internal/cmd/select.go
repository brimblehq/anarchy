package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
)

type ProjectRef struct {
	ID      string
	Name    string
	Status  string
	Project *client.Project
}

func (p ProjectRef) Display() string {
	if p.Name != "" {
		return p.Name
	}
	return p.ID
}

var objectIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{24}$`)

func isObjectID(s string) bool {
	return objectIDPattern.MatchString(s)
}

// pickProject resolves the project a command acts on: an explicit name or ID,
// else the project linked to the working directory, else an interactive picker.
func pickProject(ctx context.Context, arg string) (ProjectRef, error) {
	if arg != "" {
		return fetchProject(ctx, arg)
	}
	if linked != nil {
		return ProjectRef{ID: linked.ProjectID, Name: linked.ProjectName}, nil
	}
	if !ui.IsInteractive() {
		return ProjectRef{}, fmt.Errorf("no project given — pass a project name, or run `brimble link` in this directory")
	}

	projects, err := loadProjects(ctx)
	if err != nil {
		return ProjectRef{}, err
	}
	if len(projects) == 0 {
		return ProjectRef{}, fmt.Errorf("no projects yet — create one with `brimble project create` or `brimble deploy`")
	}
	return selectProject("Select a project", projects)
}

func selectProject(title string, projects []ProjectRef) (ProjectRef, error) {
	byID := map[string]ProjectRef{}
	opts := make([]ui.Option, 0, len(projects))
	for _, p := range projects {
		label := p.Name
		if p.Status != "" {
			label += "  " + ui.Status(p.Status)
		}
		opts = append(opts, ui.Option{Label: label, Value: p.ID})
		byID[p.ID] = p
	}
	id, err := ui.Select(title, opts)
	if err != nil {
		return ProjectRef{}, err
	}
	return byID[id], nil
}

func fetchProject(ctx context.Context, nameOrID string) (ProjectRef, error) {
	var project client.Project
	err := ui.WithSpinner("Loading "+nameOrID+"…", func() error {
		var err error
		project, err = coreClient.GetProject(ctx, nameOrID, cfg.TeamID)
		return err
	})
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return ProjectRef{}, fmt.Errorf("project %q not found%s — see `brimble project list`", nameOrID, teamSuffix())
		}
		return ProjectRef{}, err
	}
	ref := ProjectRef{ID: project.ProjectID(), Name: project.Name, Status: project.Status, Project: &project}
	if ref.ID == "" {
		return ProjectRef{}, fmt.Errorf("project %q: response has no id", nameOrID)
	}
	return ref, nil
}

func projectData(ctx context.Context, ref *ProjectRef) (client.Project, error) {
	if ref.Project != nil {
		return *ref.Project, nil
	}
	full, err := fetchProject(ctx, ref.ID)
	if err != nil {
		return client.Project{}, err
	}
	*ref = full
	return *full.Project, nil
}

func loadProjects(ctx context.Context) ([]ProjectRef, error) {
	var list client.ProjectList
	err := ui.WithSpinner("Loading projects…", func() error {
		var err error
		list, err = coreClient.ListProjects(ctx, client.ListProjectsOpts{
			TeamID: cfg.TeamID,
			Limit:  100,
			Sort:   "updatedAt",
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return projectRefs(list), nil
}

func projectRefs(list client.ProjectList) []ProjectRef {
	out := make([]ProjectRef, 0, len(list.Projects))
	for _, p := range list.Projects {
		if p.Name == "" {
			continue
		}
		out = append(out, ProjectRef{ID: p.ProjectID(), Name: p.Name, Status: p.Status})
	}
	return out
}

type teamRef struct {
	ID   string
	Name string
	Slug string
}

func loadTeams(ctx context.Context) ([]teamRef, error) {
	var teams []client.Team
	err := ui.WithSpinner("Loading teams…", func() error {
		var err error
		teams, err = coreClient.ListTeams(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return teamRefs(teams), nil
}

func teamRefs(teams []client.Team) []teamRef {
	out := make([]teamRef, 0, len(teams))
	for _, t := range teams {
		if t.Name == "" {
			continue
		}
		out = append(out, teamRef{ID: t.ID, Name: t.Name, Slug: t.Slug})
	}
	return out
}

// findTeam matches a team by ID, name or slug.
func findTeam(teams []teamRef, key string) (teamRef, bool) {
	for _, t := range teams {
		if t.ID == key || t.Slug == key || t.Name == key {
			return t, true
		}
	}
	return teamRef{}, false
}

// pickTeam resolves a team argument (ID, name or slug) or prompts for one.
func pickTeam(ctx context.Context, arg string) (teamRef, error) {
	teams, err := loadTeams(ctx)
	if err != nil {
		return teamRef{}, err
	}
	if arg != "" {
		if t, ok := findTeam(teams, arg); ok {
			return t, nil
		}
		return teamRef{}, fmt.Errorf("team %q not found — see `brimble team list`", arg)
	}
	if cfg.TeamID != "" {
		if t, ok := findTeam(teams, cfg.TeamID); ok {
			return t, nil
		}
	}
	if len(teams) == 0 {
		return teamRef{}, fmt.Errorf("you are not a member of any team")
	}
	if !ui.IsInteractive() {
		return teamRef{}, fmt.Errorf("team is required — pass a team name or set --team")
	}
	opts := make([]ui.Option, 0, len(teams))
	for _, t := range teams {
		opts = append(opts, ui.Option{Label: t.Name, Value: t.ID})
	}
	id, err := ui.Select("Select a team", opts)
	if err != nil {
		return teamRef{}, err
	}
	t, _ := findTeam(teams, id)
	return t, nil
}

func teamSuffix() string {
	if cfg.TeamID == "" {
		return " in your personal workspace"
	}
	if cfg.TeamName != "" {
		return " in team " + cfg.TeamName
	}
	return " in this team"
}

func argOr(args []string, i int) string {
	if len(args) > i {
		return args[i]
	}
	return ""
}
