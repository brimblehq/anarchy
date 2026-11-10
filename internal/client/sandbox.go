package client

import (
	"context"
	"net/http"
	"net/url"
)

// Sandbox is a minimal view of a sandbox — just enough for the SSH picker and
// CI-runner attach flows.
type Sandbox struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type SandboxSize struct {
	Name      string `json:"name"`
	VCPU      int    `json:"vcpu"`
	MemoryGB  int    `json:"memory_gb"`
	Available bool   `json:"available"`
}

// SandboxRegion is a region where sandboxes (and CI runners) can run.
type SandboxRegion struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
}

// ListSandboxes returns up to 100 sandboxes, optionally narrowed by a search
// term. It is sized for the terminal and attach pickers.
func (c *Core) ListSandboxes(ctx context.Context, search, teamID string) ([]Sandbox, error) {
	q := url.Values{}
	q.Set("limit", "100")
	if search != "" {
		q.Set("search", search)
	}
	var data struct {
		Data []Sandbox `json:"data"`
	}
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/sandboxes",
		Query:  q,
		TeamID: teamID,
	}, &data)
	return data.Data, err
}

func (c *Core) ListSandboxSizes(ctx context.Context, region, teamID string) ([]SandboxSize, error) {
	q := url.Values{}
	if region != "" {
		q.Set("region", region)
	}
	var data struct {
		Sizes []SandboxSize `json:"sizes"`
	}
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/sandboxes/sizes",
		Query:  q,
		TeamID: teamID,
	}, &data)
	return data.Sizes, err
}

func (c *Core) ListSandboxRegions(ctx context.Context, teamID string) ([]SandboxRegion, error) {
	var data struct {
		Regions []SandboxRegion `json:"regions"`
	}
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/sandboxes/regions",
		TeamID: teamID,
	}, &data)
	return data.Regions, err
}
