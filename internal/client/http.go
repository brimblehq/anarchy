package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/brimbleapp/brimble-cli/internal/auth"
)

const maxResponseBytes = 16 << 20

type HTTPClient struct {
	baseURL     string
	httpClient  *http.Client
	credentials auth.Credentials
	userAgent   string
}

type HTTPOptions struct {
	BaseURL     string
	Timeout     time.Duration
	Credentials auth.Credentials
	UserAgent   string
}

func NewHTTP(opts HTTPOptions) *HTTPClient {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ua := opts.UserAgent
	if ua == "" {
		ua = "brimble-cli"
	}
	return &HTTPClient{
		baseURL:     strings.TrimRight(opts.BaseURL, "/"),
		httpClient:  &http.Client{Timeout: timeout},
		credentials: opts.Credentials,
		userAgent:   ua,
	}
}

type APIError struct {
	Status  int
	Message string
	Body    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("api error %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("api error %d: %s", e.Status, truncate(e.Body, 300))
}

type envelope struct {
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Error   bool            `json:"error"`
}

type Request struct {
	Method  string
	Path    string
	Query   url.Values
	Body    any
	TeamID  string
	Raw     bool
	Headers map[string]string
}

func (c *HTTPClient) DoJSON(ctx context.Context, req Request, out any) (string, error) {
	if req.Method == "" {
		return "", fmt.Errorf("http method is required")
	}

	respBody, status, err := c.do(ctx, req)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", parseAPIError(status, respBody)
	}
	if len(respBody) == 0 {
		return "", nil
	}
	if req.Raw {
		if out != nil {
			if err := json.Unmarshal(respBody, out); err != nil {
				return "", fmt.Errorf("decode response: %w", err)
			}
		}
		return "", nil
	}

	var env envelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		return "", fmt.Errorf("decode envelope: %w", err)
	}
	if env.Error {
		return "", &APIError{Status: status, Message: env.Message, Body: string(respBody)}
	}
	if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return "", fmt.Errorf("decode data: %w", err)
		}
	}
	return env.Message, nil
}

func (c *HTTPClient) DoBytes(ctx context.Context, req Request) ([]byte, int, error) {
	if req.Method == "" {
		return nil, 0, fmt.Errorf("http method is required")
	}
	return c.do(ctx, req)
}

func (c *HTTPClient) do(ctx context.Context, req Request) ([]byte, int, error) {
	u, err := url.Parse(c.baseURL + req.Path)
	if err != nil {
		return nil, 0, fmt.Errorf("parse url: %w", err)
	}
	if len(req.Query) > 0 {
		u.RawQuery = req.Query.Encode()
	}

	var bodyReader io.Reader
	if req.Body != nil {
		b, err := json.Marshal(req.Body)
		if err != nil {
			return nil, 0, fmt.Errorf("encode body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, u.String(), bodyReader)
	if err != nil {
		return nil, 0, err
	}

	if req.Body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", c.userAgent)

	if c.credentials.APIKey != "" {
		httpReq.Header.Set("x-brimble-key", c.credentials.APIKey)
	}

	teamID := req.TeamID
	if teamID == "" {
		teamID = c.credentials.TeamID
	}
	if teamID != "" {
		httpReq.Header.Set("x-brimble-team", teamID)
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func parseAPIError(status int, body []byte) error {
	apiErr := &APIError{Status: status, Body: string(body)}
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Message != "" {
		apiErr.Message = payload.Message
	}
	return apiErr
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
