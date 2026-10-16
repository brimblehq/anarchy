package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/brimblehq/anarchy/internal/auth"
)

const maxResponseBytes = 16 << 20

type HTTPClient struct {
	baseURL      string
	httpClient   *http.Client
	credentials  auth.Credentials
	userAgent    string
	streamClient *http.Client // no overall Timeout; streams stay open for minutes
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
		streamClient: &http.Client{Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: timeout,
		}},
	}
}

type APIError struct {
	Status  int
	Message string
	Body    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status)
	}
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("request failed (HTTP %d)", e.Status)
	}
	return fmt.Sprintf("request failed (HTTP %d): %s", e.Status, truncate(e.Body, 300))
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

	rawBody []byte
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
	httpReq, err := c.newRequest(ctx, req)
	if err != nil {
		return nil, 0, err
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

func (c *HTTPClient) newRequest(ctx context.Context, req Request) (*http.Request, error) {
	u, err := url.Parse(c.baseURL + req.Path)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if len(req.Query) > 0 {
		u.RawQuery = req.Query.Encode()
	}

	var bodyReader io.Reader
	jsonBody := false
	if req.rawBody != nil {
		bodyReader = bytes.NewReader(req.rawBody)
	} else if req.Body != nil {
		b, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("encode body: %w", err)
		}
		// A typed nil (nil json.RawMessage, map or pointer) marshals to "null",
		// which Core's JSON parser rejects with a 500. Send no body instead.
		if !bytes.Equal(b, []byte("null")) {
			bodyReader, jsonBody = bytes.NewReader(b), true
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, u.String(), bodyReader)
	if err != nil {
		return nil, err
	}

	if jsonBody {
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
	return httpReq, nil
}

func (c *HTTPClient) openStream(ctx context.Context, req Request) (*http.Response, error) {
	httpReq, err := c.newRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	resp, err := c.streamClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, parseAPIError(resp.StatusCode, body)
	}
	return resp, nil
}

func parseAPIError(status int, body []byte) error {
	apiErr := &APIError{Status: status, Body: string(body)}
	var payload struct {
		Message string `json:"message"`
		Errors  []struct {
			Msg  string `json:"msg"`
			Path string `json:"path"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return apiErr
	}
	if payload.Message != "" {
		apiErr.Message = payload.Message
		return apiErr
	}
	// Validation failures: { errors: [{ msg, path }] }
	var parts []string
	for _, e := range payload.Errors {
		switch {
		case e.Path != "" && e.Msg != "":
			parts = append(parts, e.Path+": "+e.Msg)
		case e.Msg != "":
			parts = append(parts, e.Msg)
		}
	}
	apiErr.Message = strings.Join(parts, "; ")
	return apiErr
}

// FormFile is one file part of a multipart upload.
type FormFile struct {
	Field    string
	FileName string
	Content  io.Reader
}

// DoMultipart sends a multipart/form-data request and decodes the standard
// { message, data } envelope into out.
func (c *HTTPClient) DoMultipart(ctx context.Context, req Request, fields map[string]string, files []FormFile, out any) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if v == "" {
			continue
		}
		if err := mw.WriteField(k, v); err != nil {
			return "", err
		}
	}
	for _, f := range files {
		part, err := mw.CreateFormFile(f.Field, f.FileName)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(part, f.Content); err != nil {
			return "", err
		}
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	req.Body = nil
	req.rawBody = buf.Bytes()
	if req.Headers == nil {
		req.Headers = map[string]string{}
	}
	req.Headers["Content-Type"] = mw.FormDataContentType()
	return c.DoJSON(ctx, req, out)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
