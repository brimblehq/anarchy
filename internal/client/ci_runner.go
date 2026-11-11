package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// CI runners let a GitHub organization or a CircleCI resource class run its
// jobs on Brimble sandboxes.
//
// Two modes (CircleCI runners are always reuse):
//   - reuse:   a fixed set of attached sandboxes runs jobs (GitHub "self-hosted")
//   - per_job: a fresh sandbox is provisioned for each queued job

type CiRunner struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Team           string `json:"team"`
	Provider       string `json:"provider"`
	InstallationID int    `json:"installation_id"`
	AccountLogin   string `json:"account_login"`
	ResourceClass  string `json:"resource_class"`
	Mode           string `json:"mode"` // reuse | per_job
	Size           string `json:"size"`
	Region         string `json:"region"`
	MaxConcurrency int    `json:"max_concurrency"`
	Enabled        bool   `json:"enabled"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type CiRunnerList struct {
	Data        []CiRunner `json:"data"`
	TotalCount  int        `json:"totalCount"`
	CurrentPage int        `json:"currentPage"`
	TotalPages  int        `json:"totalPages"`
	Limit       int        `json:"limit"`
}

type CiRunnerJob struct {
	ID            string   `json:"id"`
	ExternalJobID int      `json:"external_job_id"`
	ExternalRunID int      `json:"external_run_id"`
	RepoFullName  string   `json:"repo_full_name"`
	JobName       string   `json:"job_name"`
	Labels        []string `json:"labels"`
	Size          string   `json:"size"`
	Status        string   `json:"status"`
	Conclusion    string   `json:"conclusion"`
	Error         string   `json:"error"`
	ExecutedBy    string   `json:"executed_by"`
	SandboxID     string   `json:"sandbox_id"`
	QueuedAt      string   `json:"queued_at"`
	StartedAt     string   `json:"started_at"`
	CompletedAt   string   `json:"completed_at"`
}

type CiRunnerJobList struct {
	Data        []CiRunnerJob `json:"data"`
	TotalCount  int           `json:"totalCount"`
	CurrentPage int           `json:"currentPage"`
	TotalPages  int           `json:"totalPages"`
	Limit       int           `json:"limit"`
}

type CiRunnerSandbox struct {
	ID           string `json:"id"`
	SandboxID    string `json:"sandbox_id"`
	RunnerName   string `json:"runner_name"`
	Status       string `json:"status"` // attaching | online | offline | detached
	LastOnlineAt string `json:"last_online_at"`
	Error        string `json:"error"`
	CreatedAt    string `json:"created_at"`
}

type CiRunnerJobLogs struct {
	Logs      string `json:"logs"`
	Truncated bool   `json:"truncated"`
}

type CreateCiRunnerRequest struct {
	Name           string `json:"name"`
	Provider       string `json:"provider,omitempty"`
	InstallationID int    `json:"installationId,omitempty"`
	ResourceClass  string `json:"resourceClass,omitempty"`
	Token          string `json:"token,omitempty"`
	Mode           string `json:"mode"`
	Size           string `json:"size,omitempty"`
	Region         string `json:"region,omitempty"`
	MaxConcurrency int    `json:"maxConcurrency,omitempty"`
}

type UpdateCiRunnerRequest struct {
	Name           string `json:"name,omitempty"`
	Mode           string `json:"mode,omitempty"`
	Size           string `json:"size,omitempty"`
	Region         string `json:"region,omitempty"`
	MaxConcurrency int    `json:"maxConcurrency,omitempty"`
	Enabled        *bool  `json:"enabled,omitempty"`
	Token          string `json:"token,omitempty"`
}

type NewCiRunnerSandboxInput struct {
	Name   string `json:"name,omitempty"`
	Size   string `json:"size"`
	Region string `json:"region"`
}

type AttachCiRunnerSandboxRequest struct {
	SandboxID string                   `json:"sandboxId,omitempty"`
	Sandbox   *NewCiRunnerSandboxInput `json:"sandbox,omitempty"`
}

func (c *Core) CreateCiRunner(ctx context.Context, body CreateCiRunnerRequest, teamID string) (CiRunner, error) {
	var data CiRunner
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/ci-runners",
		Body:   body,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) ListCiRunners(ctx context.Context, page, limit int, teamID string) (CiRunnerList, error) {
	q := url.Values{}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var data CiRunnerList
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/ci-runners",
		Query:  q,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) GetCiRunner(ctx context.Context, id, teamID string) (CiRunner, error) {
	var data CiRunner
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/ci-runners/" + url.PathEscape(id),
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) UpdateCiRunner(ctx context.Context, id string, body UpdateCiRunnerRequest, teamID string) (CiRunner, error) {
	var data CiRunner
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPatch,
		Path:   "/v1/ci-runners/" + url.PathEscape(id),
		Body:   body,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) DeleteCiRunner(ctx context.Context, id, teamID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodDelete,
		Path:   "/v1/ci-runners/" + url.PathEscape(id),
		TeamID: teamID,
	}, nil)
	return err
}

func (c *Core) ListCiRunnerJobs(ctx context.Context, id string, page, limit int, teamID string) (CiRunnerJobList, error) {
	q := url.Values{}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var data CiRunnerJobList
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/ci-runners/" + url.PathEscape(id) + "/jobs",
		Query:  q,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) GetCiRunnerJobLogs(ctx context.Context, id, jobID, teamID string) (CiRunnerJobLogs, error) {
	var data CiRunnerJobLogs
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/ci-runners/" + url.PathEscape(id) + "/jobs/" + url.PathEscape(jobID) + "/logs",
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) AttachCiRunnerSandbox(ctx context.Context, id string, body AttachCiRunnerSandboxRequest, teamID string) (CiRunnerSandbox, error) {
	var data CiRunnerSandbox
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/ci-runners/" + url.PathEscape(id) + "/sandboxes",
		Body:   body,
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) ListCiRunnerSandboxes(ctx context.Context, id, teamID string) ([]CiRunnerSandbox, error) {
	var data struct {
		Sandboxes []CiRunnerSandbox `json:"sandboxes"`
	}
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/ci-runners/" + url.PathEscape(id) + "/sandboxes",
		TeamID: teamID,
	}, &data)
	return data.Sandboxes, err
}

func (c *Core) DetachCiRunnerSandbox(ctx context.Context, id, sandboxID, teamID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodDelete,
		Path:   "/v1/ci-runners/" + url.PathEscape(id) + "/sandboxes/" + url.PathEscape(sandboxID),
		TeamID: teamID,
	}, nil)
	return err
}
