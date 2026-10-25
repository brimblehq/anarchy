package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type PITRStatus struct {
	Supported            bool     `json:"supported"`
	Enabled              bool     `json:"enabled"`
	Applying             bool     `json:"applying"`
	WindowDays           int      `json:"window_days"`
	DestinationType      string   `json:"destination_type"`
	EarliestRestorableAt string   `json:"earliest_restorable_at"`
	LatestRestorableAt   string   `json:"latest_restorable_at"`
	BaseBackups          []string `json:"base_backups"`
}

type BackupSettings struct {
	RetentionDays          *int              `json:"retention_days"`
	EffectiveRetentionDays *int              `json:"effective_retention_days"`
	MaxRetentionDays       int               `json:"max_retention_days"`
	CustomDestination      bool              `json:"custom_destination_enabled"`
	Destination            BackupDestination `json:"destination"`
	PITRSupported          bool              `json:"pitr_supported"`
	PITREnabled            bool              `json:"pitr_enabled"`
	PITRWindowDays         int               `json:"pitr_window_days"`
}

type BackupDestination struct {
	Type            string `json:"type"`
	BucketID        string `json:"bucket_id,omitempty"`
	Name            string `json:"name,omitempty"`
	BucketName      string `json:"bucket_name,omitempty"`
	Endpoint        string `json:"endpoint,omitempty"`
	Region          string `json:"region,omitempty"`
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
}

type UpdateBackupSettingsRequest struct {
	PITREnabled *bool              `json:"pitr_enabled,omitempty"`
	Destination *BackupDestination `json:"destination,omitempty"`
}

type PITRRestoreRequest struct {
	TargetTime string `json:"target_time"`
	Name       string `json:"name,omitempty"`
}

type PITRRestoreResult struct {
	RestoreID  string `json:"restore_id"`
	TargetTime string `json:"target_time"`
	Project    struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"project"`
}

type BackupHistoryEntry struct {
	ID                   string `json:"_id"`
	Type                 string `json:"type"`
	Status               string `json:"status"`
	Trigger              string `json:"trigger"`
	SizeBytes            int64  `json:"size_bytes"`
	TargetTime           string `json:"target_time"`
	TargetProjectID      string `json:"target_project_id"`
	TargetProjectDeleted bool   `json:"target_project_deleted"`
	FailureReason        string `json:"failure_reason"`
	CreatedAt            string `json:"created_at"`
	CompletedAt          string `json:"completed_at"`
}

type BackupHistory struct {
	Data        []BackupHistoryEntry `json:"data"`
	TotalCount  int                  `json:"totalCount"`
	CurrentPage int                  `json:"currentPage"`
	TotalPages  int                  `json:"totalPages"`
}

type StorageBucket struct {
	ID         string `json:"_id"`
	Name       string `json:"name"`
	BucketName string `json:"bucket_name"`
	Status     string `json:"status"`
}

func databasePath(projectID, rest string) string {
	return "/v1/projects/database/" + url.PathEscape(projectID) + rest
}

func (c *Core) PITRStatus(ctx context.Context, projectID, teamID string) (PITRStatus, error) {
	var data PITRStatus
	_, err := c.http.DoJSON(ctx, Request{Method: http.MethodGet, Path: databasePath(projectID, "/pitr"), TeamID: teamID}, &data)
	return data, err
}

func (c *Core) BackupSettings(ctx context.Context, projectID, teamID string) (BackupSettings, error) {
	var data BackupSettings
	_, err := c.http.DoJSON(ctx, Request{Method: http.MethodGet, Path: databasePath(projectID, "/backup-settings"), TeamID: teamID}, &data)
	return data, err
}

func (c *Core) UpdateBackupSettings(ctx context.Context, projectID string, req UpdateBackupSettingsRequest, teamID string) (BackupSettings, error) {
	var data BackupSettings
	_, err := c.http.DoJSON(ctx, Request{Method: http.MethodPatch, Path: databasePath(projectID, "/backup-settings"), Body: req, TeamID: teamID}, &data)
	return data, err
}

func (c *Core) RestoreToPointInTime(ctx context.Context, projectID string, target time.Time, name, teamID string) (PITRRestoreResult, error) {
	var data PITRRestoreResult
	body := PITRRestoreRequest{TargetTime: target.UTC().Format(time.RFC3339), Name: name}
	_, err := c.http.DoJSON(ctx, Request{Method: http.MethodPost, Path: databasePath(projectID, "/pitr/restore"), Body: body, TeamID: teamID}, &data)
	return data, err
}

func (c *Core) BackupHistory(ctx context.Context, projectID string, page, limit int, teamID string) (BackupHistory, error) {
	var data BackupHistory
	q := url.Values{"page": {strconv.Itoa(max(page, 1))}, "limit": {strconv.Itoa(limit)}}
	_, err := c.http.DoJSON(ctx, Request{Method: http.MethodGet, Path: databasePath(projectID, "/backups"), Query: q, TeamID: teamID}, &data)
	return data, err
}

func (c *Core) ListStorageBuckets(ctx context.Context, teamID string) ([]StorageBucket, error) {
	var data struct {
		Data []StorageBucket `json:"data"`
	}
	q := url.Values{"limit": {"100"}}
	_, err := c.http.DoJSON(ctx, Request{Method: http.MethodGet, Path: "/v1/storage/buckets", Query: q, TeamID: teamID}, &data)
	return data.Data, err
}
