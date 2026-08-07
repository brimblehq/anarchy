package client

import (
	"context"
	"net/http"
	"net/url"
)

func (c *Core) ListTeams(ctx context.Context) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/teams",
	}, &data)
	return data, err
}

func (c *Core) GetTeam(ctx context.Context, name string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/teams/" + url.PathEscape(name),
	}, &data)
	return data, err
}

func (c *Core) TeamMembers(ctx context.Context, name string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/teams/" + url.PathEscape(name) + "/members",
	}, &data)
	return data, err
}
