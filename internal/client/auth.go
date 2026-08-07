package client

import (
	"context"
	"net/http"
	"time"

	"github.com/brimbleapp/brimble-cli/internal/auth"
)

type Auth struct {
	http *HTTPClient
}

func NewAuth(baseURL string, timeout time.Duration, creds auth.Credentials, userAgent string) *Auth {
	return &Auth{
		http: NewHTTP(HTTPOptions{
			BaseURL:     baseURL,
			Timeout:     timeout,
			Credentials: creds,
			UserAgent:   userAgent,
		}),
	}
}

func (c *Auth) Me(ctx context.Context) (any, error) {
	var data any
	_, err := c.http.DoJSON(ctx, Request{
		Method: http.MethodGet,
		Path:   "/user/me",
	}, &data)
	return data, err
}
