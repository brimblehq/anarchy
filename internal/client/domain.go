package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

func (c *Core) ListDomains(ctx context.Context, teamID string) (DomainList, error) {
	var data DomainList
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/domains",
		Query:  teamQuery(teamID),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) DomainStatus(ctx context.Context, domain, teamID string) (json.RawMessage, error) {
	var data json.RawMessage
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/domains/" + url.PathEscape(domain) + "/status",
		Query:  teamQuery(teamID),
		TeamID: teamID,
	}, &data)
	return data, err
}

// AddDomain adds a domain to a project, or to the workspace when projectID is empty.
func (c *Core) AddDomain(ctx context.Context, projectID, name, teamID string) (json.RawMessage, error) {
	path := "/v1/domains"
	if projectID != "" {
		path += "/" + url.PathEscape(projectID)
	}
	var data json.RawMessage
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   path,
		Body: struct {
			Name   string `json:"name"`
			TeamID string `json:"teamId,omitempty"`
		}{Name: name, TeamID: teamID},
		TeamID: teamID,
	}, &data)
	return data, err
}

// DeleteDomain removes a domain from a project, or deletes it from the
// workspace when projectID is empty.
func (c *Core) DeleteDomain(ctx context.Context, domainID, projectID, teamID string) error {
	path := "/v1/domains/" + url.PathEscape(domainID)
	if projectID != "" {
		path += "/" + url.PathEscape(projectID)
	}
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodDelete,
		Path:   path,
		TeamID: teamID,
	}, nil)
	return err
}

func teamQuery(teamID string) url.Values {
	if teamID == "" {
		return nil
	}
	return url.Values{"teamId": {teamID}}
}
