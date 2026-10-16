package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type BuildLogLine struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

type buildLogFrame struct {
	Type string `json:"type"` // "line" or "end"
	BuildLogLine
	Status string `json:"status"`
}

// Core pings every 15s; this idle window means the connection is dead.
// Vars so tests can shorten them.
var (
	buildLogIdleTimeout = 45 * time.Second
	buildLogRetryDelay  = time.Second
)

const buildLogMaxRetries = 5

var errStreamDropped = errors.New("build log stream disconnected")

// StreamBuildLogs follows GET /v1/logs/stream/{projectId}/{logId} until the
// deploy ends. Dropped connections retry; Core replays on reconnect, so lines
// are de-duplicated by ID.
func (c *Core) StreamBuildLogs(ctx context.Context, projectID, logID, teamID string, onLine func(BuildLogLine)) (string, error) {
	seen := map[string]bool{}
	emit := func(l BuildLogLine) {
		if l.ID != "" {
			if seen[l.ID] {
				return
			}
			seen[l.ID] = true
		}
		onLine(l)
	}

	failures := 0
	for {
		status, progressed, err := c.streamBuildLogsOnce(ctx, projectID, logID, teamID, emit)
		if err == nil {
			return status, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			return "", err
		}
		if progressed {
			failures = 0
		}
		failures++
		if failures > buildLogMaxRetries {
			return "", fmt.Errorf("%w: %v", errStreamDropped, err)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(failures) * buildLogRetryDelay):
		}
	}
}

func (c *Core) streamBuildLogsOnce(ctx context.Context, projectID, logID, teamID string, emit func(BuildLogLine)) (string, bool, error) {
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resp, err := c.http.openStream(attemptCtx, Request{
		Method: http.MethodGet,
		Path:   "/v1/logs/stream/" + url.PathEscape(projectID) + "/" + url.PathEscape(logID),
		TeamID: teamID,
	})
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	idle := time.AfterFunc(buildLogIdleTimeout, cancel)
	defer idle.Stop()

	var status string
	var progressed bool
	err = readSSE(resp.Body, func() { idle.Reset(buildLogIdleTimeout) }, func(data string) bool {
		progressed = true
		var f buildLogFrame
		if json.Unmarshal([]byte(data), &f) != nil {
			return true
		}
		switch f.Type {
		case "line":
			emit(f.BuildLogLine)
		case "end":
			status = f.Status
			return false
		}
		return true
	})
	if status != "" {
		return status, progressed, nil
	}
	switch {
	case ctx.Err() == nil && attemptCtx.Err() != nil:
		return "", progressed, fmt.Errorf("no data for %s", buildLogIdleTimeout)
	case err != nil:
		return "", progressed, err
	}
	return "", progressed, errStreamDropped
}

func readSSE(r io.Reader, onActivity func(), onData func(string) bool) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 8<<20)
	var data []string
	for sc.Scan() {
		onActivity()
		line := strings.TrimSuffix(sc.Text(), "\r")
		switch {
		case line == "":
			if len(data) > 0 {
				if !onData(strings.Join(data, "\n")) {
					return nil
				}
				data = data[:0]
			}
		case strings.HasPrefix(line, ":"):
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	return sc.Err()
}

func IsStreamUnavailable(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusServiceUnavailable
}
