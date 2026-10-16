package client

import (
	"time"

	"github.com/brimblehq/anarchy/internal/auth"
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
