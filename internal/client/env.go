package client

import (
	"context"
	"net/http"
	"net/url"
)

func (c *Core) ListEnvVars(ctx context.Context, projectID, environment, teamID string) (any, error) {
	path := "/v1/envs/" + url.PathEscape(projectID)
	if environment != "" {
		path += "/" + url.PathEscape(environment)
	}
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   path,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) SetEnvVar(ctx context.Context, projectID, environment string, body any, teamID string) (any, error) {
	path := "/v1/envs/" + url.PathEscape(projectID)
	if environment != "" {
		path += "/" + url.PathEscape(environment)
	}
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   path,
		Body:   body,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) DeleteEnvVar(ctx context.Context, projectID, envID, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodDelete,
		Path:   "/v1/envs/" + url.PathEscape(projectID) + "/" + url.PathEscape(envID),
		TeamID: teamID,
	}, &data)
	return data, err
}
