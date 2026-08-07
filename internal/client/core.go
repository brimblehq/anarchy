package client

import (
	"time"

	"github.com/brimbleapp/brimble-cli/internal/auth"
)

type Core struct {
	http *HTTPClient
}

func NewCore(baseURL string, timeout time.Duration, creds auth.Credentials, userAgent string) *Core {
	return &Core{
		http: NewHTTP(HTTPOptions{
			BaseURL:     baseURL,
			Timeout:     timeout,
			Credentials: creds,
			UserAgent:   userAgent,
		}),
	}
}

func (c *Core) HTTP() *HTTPClient {
	return c.http
}
