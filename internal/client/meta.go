package client

import (
	"context"
	"encoding/json"
	"net/http"
)

func (c *Core) GetRegions(ctx context.Context) (RegionCatalog, error) {
	var data RegionCatalog
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/regions",
	}, &data)
	return data, err
}

func (c *Core) GetPlans(ctx context.Context) (json.RawMessage, error) {
	var data json.RawMessage
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/plans",
	}, &data)
	return data, err
}

func (c *Core) GetFrameworks(ctx context.Context) ([]Framework, error) {
	var data []Framework
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/frameworks",
	}, &data)
	return data, err
}
