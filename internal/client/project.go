package client

import (
	"context"
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

func (c *Core) ListProjects(ctx context.Context, opts ListProjectsOpts) (any, error) {
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

	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/projects",
		Query:  q,
		TeamID: opts.TeamID,
	}, &data)
	return data, err
}

func (c *Core) GetProject(ctx context.Context, nameOrID, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/projects/" + url.PathEscape(nameOrID),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) CreateProject(ctx context.Context, body any, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects",
		Body:   body,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) RedeployProject(ctx context.Context, projectID string, body any, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/" + url.PathEscape(projectID) + "/redeploy",
		Body:   body,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) CancelDeploy(ctx context.Context, projectID, logID, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/cancel/" + url.PathEscape(projectID) + "/" + url.PathEscape(logID),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) DeleteProject(ctx context.Context, projectID, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodDelete,
		Path:   "/v1/projects/" + url.PathEscape(projectID),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) LinkProject(ctx context.Context, projectID string, body any, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/link/" + url.PathEscape(projectID),
		Body:   body,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) UnlinkProject(ctx context.Context, projectID string, body any, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/projects/unlink/" + url.PathEscape(projectID),
		Body:   body,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) ProjectStats(ctx context.Context, projectID, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/projects/stats/" + url.PathEscape(projectID),
		TeamID: teamID,
	}, &data)
	return data, err
}
