package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

func (c *Core) ListGitAccounts(ctx context.Context, git, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/accounts/" + url.PathEscape(git),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) ListRepos(ctx context.Context, git string, query url.Values, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/repos/" + url.PathEscape(git),
		Query:  query,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) GetRepo(ctx context.Context, git string, query url.Values, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/repos/" + url.PathEscape(git) + "/repo",
		Query:  query,
		TeamID: teamID,
	}, &data)
	return data, err
}

func InstallationQuery(installationID int) url.Values {
	q := url.Values{}
	if installationID > 0 {
		q.Set("installationId", strconv.Itoa(installationID))
	}
	return q
}
