package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Environments group projects (production, staging, …). Not to be confused
// with a project's secrets, which live under /v1/envs.

func (c *Core) ListEnvironments(ctx context.Context, teamID string) ([]Environment, error) {
	var data []Environment
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/environments",
		Query:  teamQuery(teamID),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) CreateEnvironment(ctx context.Context, name, inheritFrom, teamID string) (json.RawMessage, error) {
	var data json.RawMessage
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/environments",
		Body: struct {
			Name        string `json:"name"`
			InheritFrom string `json:"inheritFrom,omitempty"`
			TeamID      string `json:"teamId,omitempty"`
		}{Name: name, InheritFrom: inheritFrom, TeamID: teamID},
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) DeleteEnvironment(ctx context.Context, environmentID, moveTo, teamID string) error {
	q := url.Values{"moveTo": {moveTo}}
	if teamID != "" {
		q.Set("teamId", teamID)
	}
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodDelete,
		Path:   "/v1/environments/" + url.PathEscape(environmentID),
		Query:  q,
		TeamID: teamID,
	}, nil)
	return err
}

func (c *Core) SetDefaultEnvironment(ctx context.Context, environmentID, teamID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPatch,
		Path:   "/v1/environments/" + url.PathEscape(environmentID) + "/default",
		Query:  teamQuery(teamID),
		TeamID: teamID,
	}, nil)
	return err
}
