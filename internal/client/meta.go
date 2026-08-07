package client

import (
	"context"
	"net/http"
)

func (c *Core) GetRegions(ctx context.Context) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/regions",
	}, &data)
	return data, err
}

func (c *Core) GetPlans(ctx context.Context) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/plans",
	}, &data)
	return data, err
}

func (c *Core) GetFrameworks(ctx context.Context) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/frameworks",
	}, &data)
	return data, err
}
