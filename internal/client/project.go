package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type ListProjectsOpts struct {
	TeamID        string
	EnvironmentID string
	ServiceType   string
	Status        string
	Sort          string
	Page          int
	Limit         int
}

func (c *Core) ListProjects(ctx context.Context, opts ListProjectsOpts) (ProjectList, error) {
	q := url.Values{}
	if opts.TeamID != "" {
		q.Set("teamId", opts.TeamID)
	}
	if opts.EnvironmentID != "" {
		q.Set("environmentId", opts.EnvironmentID)
	}
	if opts.ServiceType != "" {
		q.Set("serviceType", opts.ServiceType)
	}
	if opts.Status != "" {
		q.Set("status", opts.Status)
	}
	if opts.Sort != "" {
		q.Set("sort", opts.Sort)
	}
	if opts.Page > 0 {
		q.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}

	var data ProjectList
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/projects",
		Query:  q,
		TeamID: opts.TeamID,
	}, &data)
	return data, err
}

func (c *Core) GetProject(ctx context.Context, nameOrID, teamID string) (Project, error) {
	var data Project
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/projects/" + url.PathEscape(nameOrID),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) CreateProject(ctx context.Context, body json.RawMessage, teamID string) (CreateResult, error) {
	var data CreateResult
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects",
		Body:   body,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) RedeployProject(ctx context.Context, projectID string, body json.RawMessage, teamID string) (DeployResult, error) {
	var data DeployResult
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/" + url.PathEscape(projectID) + "/redeploy",
		Body:   body,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) CancelDeploy(ctx context.Context, projectID, logID, teamID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/cancel/" + url.PathEscape(projectID) + "/" + url.PathEscape(logID),
		TeamID: teamID,
	}, nil)
	return err
}

func (c *Core) DeleteProject(ctx context.Context, projectID string, deleteVolume bool, teamID string) error {
	body := struct {
		DeleteVolume bool   `json:"deleteVolume"`
		TeamID       string `json:"teamId,omitempty"`
	}{DeleteVolume: deleteVolume, TeamID: teamID}
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodDelete,
		Path:   "/v1/projects/" + url.PathEscape(projectID),
		Body:   body,
		TeamID: teamID,
	}, nil)
	return err
}

func (c *Core) LinkProject(ctx context.Context, projectID string, body json.RawMessage, teamID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/link/" + url.PathEscape(projectID),
		Body:   body,
		TeamID: teamID,
	}, nil)
	return err
}

func (c *Core) UnlinkProject(ctx context.Context, projectID, teamID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/unlink/" + url.PathEscape(projectID),
		Body:   LinkRepoRequest{Type: "repo"},
		TeamID: teamID,
	}, nil)
	return err
}

func (c *Core) ProjectStats(ctx context.Context, projectID, teamID string) (ProjectStats, error) {
	var data ProjectStats
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/projects/stats/" + url.PathEscape(projectID),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) DatabaseInsights(ctx context.Context, projectID, teamID string) (DatabaseInsights, error) {
	var data DatabaseInsights
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/projects/database/" + url.PathEscape(projectID) + "/insights",
		TeamID: teamID,
	}, &data)
	return data, err
}

// Rollback puts an earlier deployment's image and environment variables back
// live without rebuilding. Core pauses auto-deploy on push until
// ResumeAutoDeploy or the next manual deploy.
func (c *Core) Rollback(ctx context.Context, projectID, logID, teamID string) (RollbackResult, error) {
	var data RollbackResult
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/" + url.PathEscape(projectID) + "/rollback/" + url.PathEscape(logID),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) ResumeAutoDeploy(ctx context.Context, projectID, teamID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/" + url.PathEscape(projectID) + "/auto-deploy/resume",
		TeamID: teamID,
	}, nil)
	return err
}

// DuplicateProject copies a project, its variables and domains into another
// environment and starts its first deploy.
func (c *Core) DuplicateProject(ctx context.Context, projectID string, req DuplicateProjectRequest, teamID string) (DuplicateProjectResult, error) {
	var data DuplicateProjectResult
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/" + url.PathEscape(projectID) + "/duplicate",
		Body:   req,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) StartProject(ctx context.Context, projectID, teamID string) (json.RawMessage, error) {
	return c.projectAction(ctx, projectID, "start", teamID)
}

func (c *Core) StopProject(ctx context.Context, projectID, teamID string) (json.RawMessage, error) {
	return c.projectAction(ctx, projectID, "stop", teamID)
}

func (c *Core) projectAction(ctx context.Context, projectID, action, teamID string) (json.RawMessage, error) {
	var data json.RawMessage
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/" + url.PathEscape(projectID) + "/" + action,
		Body:   teamBody(teamID),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) SetMaintenance(ctx context.Context, projectID string, enabled bool, teamID string) error {
	return c.putProjectSetting(ctx, "maintenance", projectID, struct {
		Maintenance bool `json:"maintenance"`
	}{enabled}, teamID)
}

func (c *Core) SetPasswordProtection(ctx context.Context, projectID string, enabled bool, password, teamID string) error {
	body := struct {
		PasswordEnabled bool   `json:"passwordEnabled"`
		Password        string `json:"password,omitempty"`
		TeamID          string `json:"teamId,omitempty"`
	}{PasswordEnabled: enabled, Password: password, TeamID: teamID}
	return c.putProjectSetting(ctx, "password-protect", projectID, body, teamID)
}

func (c *Core) SetPreviewDeployments(ctx context.Context, projectID string, enabled bool, teamID string) error {
	return c.putProjectSetting(ctx, "preview-deployments", projectID, struct {
		Enabled bool `json:"enabled"`
	}{enabled}, teamID)
}

func (c *Core) putProjectSetting(ctx context.Context, setting, projectID string, body any, teamID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPut,
		Path:   "/v1/projects/" + setting + "/" + url.PathEscape(projectID),
		Body:   body,
		TeamID: teamID,
	}, nil)
	return err
}

func (c *Core) TransferProject(ctx context.Context, projectID, toTeamID, teamID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/transfer/" + url.PathEscape(projectID),
		Body: struct {
			TeamID string `json:"teamId"`
		}{toTeamID},
		TeamID: teamID,
	}, nil)
	return err
}

func (c *Core) ValidateImage(ctx context.Context, image, username, token, teamID string) (bool, error) {
	body := struct {
		ImageURI    string            `json:"image_uri"`
		Credentials map[string]string `json:"credentials,omitempty"`
	}{ImageURI: image}
	if username != "" && token != "" {
		body.Credentials = map[string]string{"username": username, "token": token}
	}
	var data struct {
		IsValid bool `json:"isValid"`
	}
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/validate-image",
		Body:   body,
		TeamID: teamID,
	}, &data)
	return data.IsValid, err
}

// DropOpts uploads a zip archive as a static (drop) project. With ProjectID
// set it replaces the files of that project; otherwise it creates Name.
type DropOpts struct {
	ProjectID     string
	Name          string
	TeamID        string
	EnvironmentID string
	Archive       io.Reader
}

func (c *Core) DropUpload(ctx context.Context, opts DropOpts) (DeployResult, error) {
	path := "/v1/projects/drop"
	fields := map[string]string{}
	if opts.ProjectID != "" {
		path = "/v1/projects/" + url.PathEscape(opts.ProjectID) + "/drop"
	} else {
		fields["name"] = opts.Name
		fields["teamId"] = opts.TeamID
		fields["environmentId"] = opts.EnvironmentID
	}
	var data DeployResult
	_, err := c.http.DoMultipart(ctx, Request{
		Method: http.MethodPost,
		Path:   path,
		TeamID: opts.TeamID,
	}, fields, []FormFile{{Field: "file", FileName: "site.zip", Content: opts.Archive}}, &data)
	return data, err
}

type teamIDBody struct {
	TeamID string `json:"teamId"`
}

func teamBody(teamID string) *teamIDBody {
	if teamID == "" {
		return nil
	}
	return &teamIDBody{TeamID: teamID}
}
