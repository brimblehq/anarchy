package client

import (
	"context"
	"net/http"
	"net/url"
)

func (c *Core) ListTeams(ctx context.Context) ([]Team, error) {
	var data []Team
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/teams",
	}, &data)
	return data, err
}

func (c *Core) GetTeam(ctx context.Context, name string) (Team, error) {
	var data Team
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/teams/" + url.PathEscape(name),
	}, &data)
	return data, err
}

func (c *Core) TeamMembers(ctx context.Context, name string) ([]TeamMember, error) {
	var data []TeamMember
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/teams/" + url.PathEscape(name) + "/members",
	}, &data)
	return data, err
}

func (c *Core) InviteMembers(ctx context.Context, teamID string, emails []string, role string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/teams/" + url.PathEscape(teamID) + "/invite",
		Body: struct {
			Members []string `json:"members"`
			Role    string   `json:"role,omitempty"`
		}{Members: emails, Role: role},
		TeamID: teamID,
	}, nil)
	return err
}

func (c *Core) RemoveMember(ctx context.Context, teamID, memberID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/teams/" + url.PathEscape(teamID) + "/members/" + url.PathEscape(memberID) + "/remove",
		TeamID: teamID,
	}, nil)
	return err
}
