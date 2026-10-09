package api

import (
	"context"
	"strings"
	"time"
)

// App is one of the user's apps.
type App struct {
	ID                string
	Name              string
	Subdomain         string
	Description       string
	RepoURL           string
	DefaultBranch     string
	AutoDeployEnabled bool
	AutoDeployBranch  string
	// StoppedAt is set when the app has been stopped, by the user or by the
	// platform (StoppedBy). It is the desired state, not whether the
	// container runs right now.
	StoppedAt  *time.Time
	StoppedBy  string
	InsertedAt time.Time
	UpdatedAt  time.Time
}

type appAttributes struct {
	Name              string   `json:"name"`
	Subdomain         string   `json:"subdomain"`
	Description       *string  `json:"description"`
	RepoURL           string   `json:"repo_url"`
	DefaultBranch     *string  `json:"default_branch"`
	AutoDeployEnabled bool     `json:"auto_deploy_enabled"`
	AutoDeployBranch  *string  `json:"auto_deploy_branch"`
	StoppedAt         *apiTime `json:"stopped_at"`
	StoppedBy         *string  `json:"stopped_by"`
	InsertedAt        apiTime  `json:"inserted_at"`
	UpdatedAt         apiTime  `json:"updated_at"`
}

// Apps returns the user's apps, newest first.
func (c *Client) Apps(ctx context.Context) ([]App, error) {
	var doc document[[]resource[appAttributes], struct{}]
	if err := c.get(ctx, "apps", &doc); err != nil {
		return nil, err
	}

	apps := make([]App, 0, len(doc.Data))
	for _, r := range doc.Data {
		a := r.Attributes
		app := App{
			ID:                r.ID,
			Name:              a.Name,
			Subdomain:         a.Subdomain,
			Description:       deref(a.Description),
			RepoURL:           a.RepoURL,
			DefaultBranch:     deref(a.DefaultBranch),
			AutoDeployEnabled: a.AutoDeployEnabled,
			AutoDeployBranch:  deref(a.AutoDeployBranch),
			StoppedBy:         deref(a.StoppedBy),
			InsertedAt:        a.InsertedAt.Time,
			UpdatedAt:         a.UpdatedAt.Time,
		}
		if a.StoppedAt != nil {
			stopped := a.StoppedAt.Time
			app.StoppedAt = &stopped
		}
		apps = append(apps, app)
	}
	return apps, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// apiTime reads the API's timestamps. They are RFC 3339, but versions of the
// API before October 2026 sent some in UTC without a time zone, such as
// "2026-09-27T13:27:44".
type apiTime struct{ time.Time }

func (t *apiTime) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if s == "null" || s == "" {
		return nil
	}

	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999"} {
		if parsed, err := time.Parse(layout, s); err == nil {
			t.Time = parsed.UTC()
			return nil
		}
	}
	return &time.ParseError{Layout: time.RFC3339, Value: s, Message: ": unknown timestamp format"}
}
