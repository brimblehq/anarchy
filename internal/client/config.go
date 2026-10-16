package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Config as code: brimble.toml in the repository describes a project, and
// Core applies it on every push. These endpoints back the dashboard's
// Configuration → Config as code screen.

// ConfigIssue is one problem found in a brimble.toml.
type ConfigIssue struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// ConfigChange is one field that a plan changes.
type ConfigChange struct {
	Path   string          `json:"path"`
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after,omitempty"`
	Kind   string          `json:"kind"`
	Reason string          `json:"reason"`
}

// ConfigStep is what a plan does to one resource. Kind is noop, safe
// (applies automatically), create or confirm (need approval) or blocked.
type ConfigStep struct {
	ID       string         `json:"id"`
	Resource string         `json:"resource"`
	Name     string         `json:"name"`
	Kind     string         `json:"kind"`
	Changes  []ConfigChange `json:"changes"`
	Reason   string         `json:"reason"`
}

type ConfigPlanBody struct {
	Steps    []ConfigStep `json:"steps"`
	Orphaned []struct {
		Resource string `json:"resource"`
		Name     string `json:"name"`
	} `json:"orphaned"`
	Errors []ConfigIssue `json:"errors"`
}

// ConfigPlan is a recorded comparison of a brimble.toml with what's running.
// Status is planned, awaiting_approval, applying, applied, failed or rejected.
type ConfigPlan struct {
	ID        string         `json:"_id"`
	Status    string         `json:"status"`
	Commit    string         `json:"commit"`
	Path      string         `json:"path"`
	Log       string         `json:"log"`
	Plan      ConfigPlanBody `json:"plan"`
	Error     string         `json:"error"`
	CreatedAt string         `json:"createdAt"`
	// Errors is set instead of a plan when the submitted file is invalid.
	Errors []ConfigIssue `json:"errors"`
}

// ExportConfig generates a brimble.toml from the project's current settings.
// Environment variable values are left out.
func (c *Core) ExportConfig(ctx context.Context, projectID, teamID string) (string, error) {
	var data struct {
		Content string `json:"content"`
	}
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/config/" + url.PathEscape(projectID) + "/export",
		TeamID: teamID,
	}, &data)
	return data.Content, err
}

// ValidateConfig checks a brimble.toml without touching any project.
func (c *Core) ValidateConfig(ctx context.Context, content, teamID string) ([]ConfigIssue, error) {
	var data struct {
		Errors []ConfigIssue `json:"errors"`
	}
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/config/validate",
		Body:   map[string]string{"content": content},
		TeamID: teamID,
	}, &data)
	return data.Errors, err
}

// PlanConfig compares a brimble.toml with the project and records a plan
// awaiting approval. Nothing changes until it's approved.
func (c *Core) PlanConfig(ctx context.Context, projectID, content, teamID string) (ConfigPlan, error) {
	var data ConfigPlan
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/config/" + url.PathEscape(projectID) + "/plan",
		Body:   map[string]string{"content": content},
		TeamID: teamID,
	}, &data)
	return data, err
}

// LatestConfigPlan returns the project's most recent plan, or nil when none.
func (c *Core) LatestConfigPlan(ctx context.Context, projectID, teamID string) (*ConfigPlan, error) {
	var data *ConfigPlan
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v1/config/" + url.PathEscape(projectID) + "/plan",
		TeamID: teamID,
	}, &data)
	return data, err
}

// ApproveConfigPlan applies a plan; a deploy paused on it then resumes.
func (c *Core) ApproveConfigPlan(ctx context.Context, applyID, projectID, teamID string) error {
	return c.decideConfigPlan(ctx, applyID, projectID, teamID, "approve")
}

// RejectConfigPlan discards a plan; a deploy paused on it is cancelled.
func (c *Core) RejectConfigPlan(ctx context.Context, applyID, projectID, teamID string) error {
	return c.decideConfigPlan(ctx, applyID, projectID, teamID, "reject")
}

func (c *Core) decideConfigPlan(ctx context.Context, applyID, projectID, teamID, decision string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/config/applies/" + url.PathEscape(applyID) + "/" + decision,
		Body:   map[string]string{"projectId": projectID},
		TeamID: teamID,
	}, nil)
	return err
}
