package api

import (
	"context"
	"time"
)

// User is the user behind the API key.
type User struct {
	ID     string
	Email  string
	APIKey APIKey
}

// APIKey describes the key used for the request.
type APIKey struct {
	Name      string
	Scopes    []string
	ExpiresAt time.Time
}

type userAttributes struct {
	Email string `json:"email"`
}

type meMeta struct {
	APIKey struct {
		Name      string    `json:"name"`
		Scopes    []string  `json:"scopes"`
		ExpiresAt time.Time `json:"expires_at"`
	} `json:"api_key"`
}

// Me returns the user behind the API key, and the key itself.
func (c *Client) Me(ctx context.Context) (User, error) {
	var doc document[resource[userAttributes], meMeta]
	if err := c.get(ctx, []string{"me"}, nil, &doc); err != nil {
		return User{}, err
	}

	return User{
		ID:    doc.Data.ID,
		Email: doc.Data.Attributes.Email,
		APIKey: APIKey{
			Name:      doc.Meta.APIKey.Name,
			Scopes:    doc.Meta.APIKey.Scopes,
			ExpiresAt: doc.Meta.APIKey.ExpiresAt,
		},
	}, nil
}
