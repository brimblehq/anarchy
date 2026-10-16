package client

import (
	"context"
	"net/http"
	"net/url"
)

func (c *Core) ListGitAccounts(ctx context.Context, git, teamID string) (GitAccounts, error) {
	var data GitAccounts
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/accounts/" + url.PathEscape(git),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) ListRepos(ctx context.Context, git string, query url.Values, teamID string) (RepoList, error) {
	var data RepoList
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/repos/" + url.PathEscape(git),
		Query:  query,
		TeamID: teamID,
	}, &data)
	return data, err
}
