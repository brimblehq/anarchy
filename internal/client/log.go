package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

func (c *Core) ListDeployLogs(ctx context.Context, projectID, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/logs/" + url.PathEscape(projectID),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) SearchDeployLogs(ctx context.Context, projectID string, query url.Values, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/logs/search/" + url.PathEscape(projectID),
		Query:  query,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) RequestLogs(ctx context.Context, projectID, teamID string) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/logs/requests/" + url.PathEscape(projectID),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) DownloadLog(ctx context.Context, projectID, logID, teamID string) ([]byte, error) {
	body, status, err := c.http.DoBytes(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/logs/download/" + url.PathEscape(projectID) + "/" + url.PathEscape(logID),
		TeamID: teamID,
	})
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, parseAPIError(status, body)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("empty log download")
	}
	return body, nil
}
