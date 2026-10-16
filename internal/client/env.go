package client

import (
	"context"
	"net/http"
	"net/url"
)

// EnvVar is a secret name/value pair as Core accepts it (SecretInput).
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func envsPath(projectID, environment string) string {
	path := "/v1/envs/" + url.PathEscape(projectID)
	if environment != "" {
		path += "/" + url.PathEscape(environment)
	}
	return path
}

// ListEnvVars lists a project's secrets. environment is "", PRODUCTION or PREVIEW.
// Values come back encrypted; use Decrypt to read them.
func (c *Core) ListEnvVars(ctx context.Context, projectID, environment, teamID string) (EnvList, error) {
	q := url.Values{}
	q.Set("limit", "500")
	var data EnvList
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   envsPath(projectID, environment),
		Query:  q,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) CreateEnvVars(ctx context.Context, projectID, environment string, vars []EnvVar, teamID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   envsPath(projectID, environment),
		Body: struct {
			Environments []EnvVar `json:"environments"`
		}{vars},
		TeamID: teamID,
	}, nil)
	return err
}

func (c *Core) UpdateEnvVar(ctx context.Context, projectID, envID string, v EnvVar, teamID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPatch,
		Path:   "/v1/envs/" + url.PathEscape(projectID) + "/" + url.PathEscape(envID),
		Body: struct {
			Environment EnvVar `json:"environment"`
		}{v},
		TeamID: teamID,
	}, nil)
	return err
}

func (c *Core) DeleteEnvVar(ctx context.Context, projectID, envID, teamID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodDelete,
		Path:   "/v1/envs/" + url.PathEscape(projectID) + "/" + url.PathEscape(envID),
		TeamID: teamID,
	}, nil)
	return err
}

// Decrypt turns encrypted secret values (as returned by ListEnvVars) into plain text.
func (c *Core) Decrypt(ctx context.Context, vars []EnvVar, teamID string) ([]EnvVar, error) {
	var data []EnvVar
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/decrypt",
		Body: struct {
			Environments []EnvVar `json:"environments"`
		}{vars},
		TeamID: teamID,
	}, &data)
	return data, err
}
