package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
)

// Resource kinds accepted by the terminal-session endpoints. These are the
// URL path segments used by Core ("sandboxes" / "projects").
const (
	ResourceSandbox = "sandboxes"
	ResourceProject = "projects"
)

type TerminalSession struct {
	SessionID string `json:"session_id"`
}

// CreateTerminalSession opens a terminal session for a sandbox or project.
// resource is ResourceSandbox or ResourceProject.
func (c *Core) CreateTerminalSession(ctx context.Context, resource, id, cwd, teamID string) (TerminalSession, error) {
	var data TerminalSession
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodPost,
		Path:   "/v1/" + resource + "/" + url.PathEscape(id) + "/terminal/sessions",
		Body: struct {
			Cwd string `json:"cwd,omitempty"`
		}{cwd},
		TeamID: teamID,
	}, &data)
	return data, err
}

func (c *Core) CloseTerminalSession(ctx context.Context, resource, id, sessionID, teamID string) error {
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodDelete,
		Path:   "/v1/" + resource + "/" + url.PathEscape(id) + "/terminal/sessions/" + url.PathEscape(sessionID),
		TeamID: teamID,
	}, nil)
	return err
}

// TerminalConn is a live PTY connection to a terminal session. Bytes written
// with WriteStdin go to the shell; messages read with ReadMessage are the
// shell's stdout.
type TerminalConn struct {
	ws *websocket.Conn
}

func (t *TerminalConn) ReadMessage() ([]byte, error) {
	_, msg, err := t.ws.ReadMessage()
	return msg, err
}

func (t *TerminalConn) WriteStdin(p []byte) error {
	return t.ws.WriteMessage(websocket.BinaryMessage, p)
}

func (t *TerminalConn) Resize(cols, rows int) error {
	payload, err := json.Marshal(resizeMessage{Type: "resize", Cols: cols, Rows: rows})
	if err != nil {
		return err
	}
	return t.ws.WriteMessage(websocket.TextMessage, payload)
}

func (t *TerminalConn) Close() error {
	return t.ws.Close()
}

type resizeMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// OpenTerminal attaches to a terminal session's PTY over a raw WebSocket.
// cols/rows are the initial terminal size.
func (c *Core) OpenTerminal(ctx context.Context, resource, id, sessionID string, cols, rows int) (*TerminalConn, error) {
	wsURL, err := terminalPTYURL(c.http.baseURL, resource, id, sessionID, cols, rows)
	if err != nil {
		return nil, err
	}

	headers := http.Header{}
	if c.http.credentials.APIKey != "" {
		headers.Set("x-brimble-key", c.http.credentials.APIKey)
	}

	ws, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusForbidden {
				return nil, fmt.Errorf("your API key can't open a terminal on this resource (missing exec permission)")
			}
		}
		return nil, fmt.Errorf("connect to terminal: %w", err)
	}
	return &TerminalConn{ws: ws}, nil
}

// terminalPTYURL builds the WebSocket URL Core's PTY gateway accepts, e.g.
// wss://api.brimble.io/core/ws/sandboxes/:id/terminal/sessions/:sid/pty.
func terminalPTYURL(coreBaseURL, resource, id, sessionID string, cols, rows int) (string, error) {
	if coreBaseURL == "" {
		return "", fmt.Errorf("core base URL is empty")
	}
	u, err := url.Parse(coreBaseURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid core base URL %q", coreBaseURL)
	}

	scheme := "wss"
	if u.Scheme == "http" {
		scheme = "ws"
	}
	prefix := strings.TrimSuffix(u.Path, "/")
	path := prefix + "/ws/" + resource + "/" + url.PathEscape(id) + "/terminal/sessions/" + url.PathEscape(sessionID) + "/pty"

	q := url.Values{}
	q.Set("cols", strconv.Itoa(cols))
	q.Set("rows", strconv.Itoa(rows))
	return scheme + "://" + u.Host + path + "?" + q.Encode(), nil
}
