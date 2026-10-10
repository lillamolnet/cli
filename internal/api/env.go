package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// EnvVar is an app's environment variable. The API never gives the value.
type EnvVar struct {
	ID            string
	Key           string
	Scope         string
	Secret        bool
	LastRotatedAt *time.Time
	InsertedAt    time.Time
	UpdatedAt     time.Time
}

// The results of setting a variable.
const (
	EnvCreated = "created"
	EnvUpdated = "updated"
	EnvRotated = "rotated"
)

// SetEnvVar is a variable to set. Empty Scope and nil Secret keep what the
// variable has, or use the API's defaults for a new one.
type SetEnvVar struct {
	Value  string
	Scope  string
	Secret *bool
}

type envVarAttributes struct {
	Key           string   `json:"key"`
	Scope         string   `json:"scope"`
	IsSecret      bool     `json:"is_secret"`
	LastRotatedAt *apiTime `json:"last_rotated_at"`
	InsertedAt    apiTime  `json:"inserted_at"`
	UpdatedAt     apiTime  `json:"updated_at"`
}

func toEnvVar(r resource[envVarAttributes]) EnvVar {
	a := r.Attributes
	return EnvVar{
		ID:            r.ID,
		Key:           a.Key,
		Scope:         a.Scope,
		Secret:        a.IsSecret,
		LastRotatedAt: timePtr(a.LastRotatedAt),
		InsertedAt:    a.InsertedAt.Time,
		UpdatedAt:     a.UpdatedAt.Time,
	}
}

// EnvVars returns an app's environment variables by key, without values.
func (c *Client) EnvVars(ctx context.Context, appID string) ([]EnvVar, error) {
	var doc document[[]resource[envVarAttributes], struct{}]
	if err := c.get(ctx, []string{"apps", appID, "env"}, nil, &doc); err != nil {
		return nil, err
	}

	vars := make([]EnvVar, 0, len(doc.Data))
	for _, r := range doc.Data {
		vars = append(vars, toEnvVar(r))
	}
	return vars, nil
}

// SetEnvVar sets the variable with the key, and says whether it was
// created, updated or rotated. It takes effect at the next deploy.
func (c *Client) SetEnvVar(ctx context.Context, appID, key string, v SetEnvVar) (EnvVar, string, error) {
	attributes := map[string]any{"value": v.Value}
	if v.Scope != "" {
		attributes["scope"] = v.Scope
	}
	if v.Secret != nil {
		attributes["is_secret"] = *v.Secret
	}
	body, err := json.Marshal(map[string]any{"data": map[string]any{"type": "env_var", "attributes": attributes}})
	if err != nil {
		return EnvVar{}, "", err
	}

	var doc document[resource[envVarAttributes], struct {
		Result string `json:"result"`
	}]
	r := request{method: http.MethodPut, path: []string{"apps", appID, "env", key}, body: body}
	if err := c.document(ctx, r, &doc); err != nil {
		return EnvVar{}, "", err
	}
	return toEnvVar(doc.Data), doc.Meta.Result, nil
}

// UnsetEnvVar removes the variable with the key. It takes effect at the next
// deploy.
func (c *Client) UnsetEnvVar(ctx context.Context, appID, key string) error {
	_, err := c.fetch(ctx, request{
		method: http.MethodDelete,
		path:   []string{"apps", appID, "env", key},
		accept: mediaType,
		limit:  maxDocumentSize,
	})
	return err
}
