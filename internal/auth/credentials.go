package auth

type Credentials struct {
	APIKey string
	TeamID string
}

func (c Credentials) HasAPIKey() bool {
	return c.APIKey != ""
}
