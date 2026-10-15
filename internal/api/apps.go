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
	URL               string
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
	// Settings are zero from a version of the API before October 2026, which
	// did not have them.
	Settings Settings
}

// Settings are how the platform runs an app: the port it sends traffic to,
// the health check and the resource limits. The user cannot change the
// limits.
type Settings struct {
	// ContainerPort is the port the app must listen on, and the value of PORT.
	ContainerPort int
	// HealthCheckType is http or tcp.
	HealthCheckType string
	// HealthCheckPath is empty when the http check asks for /.
	HealthCheckPath string
	// HealthCheckPort is the port of a tcp check, or zero for ContainerPort.
	HealthCheckPort int
	MemoryLimitMB   int
	CPULimit        float64
	PidsLimit       int
}

type appAttributes struct {
	Name              string   `json:"name"`
	Subdomain         string   `json:"subdomain"`
	URL               string   `json:"url"`
	Description       *string  `json:"description"`
	RepoURL           string   `json:"repo_url"`
	DefaultBranch     *string  `json:"default_branch"`
	AutoDeployEnabled bool     `json:"auto_deploy_enabled"`
	AutoDeployBranch  *string  `json:"auto_deploy_branch"`
	StoppedAt         *apiTime `json:"stopped_at"`
	StoppedBy         *string  `json:"stopped_by"`
	InsertedAt        apiTime  `json:"inserted_at"`
	UpdatedAt         apiTime  `json:"updated_at"`
	ContainerPort     int      `json:"container_port"`
	HealthCheckType   string   `json:"health_check_type"`
	HealthCheckPath   *string  `json:"health_check_path"`
	HealthCheckPort   *int     `json:"health_check_port"`
	MemoryLimitMB     int      `json:"memory_limit_mb"`
	CPULimit          float64  `json:"cpu_limit"`
	PidsLimit         int      `json:"pids_limit"`
}

// Apps returns the user's apps, newest first.
func (c *Client) Apps(ctx context.Context) ([]App, error) {
	var doc document[[]resource[appAttributes], struct{}]
	if err := c.get(ctx, []string{"apps"}, nil, &doc); err != nil {
		return nil, err
	}

	apps := make([]App, 0, len(doc.Data))
	for _, r := range doc.Data {
		apps = append(apps, toApp(r))
	}
	return apps, nil
}

func toApp(r resource[appAttributes]) App {
	a := r.Attributes
	app := App{
		ID:                r.ID,
		Name:              a.Name,
		Subdomain:         a.Subdomain,
		URL:               a.URL,
		Description:       deref(a.Description),
		RepoURL:           a.RepoURL,
		DefaultBranch:     deref(a.DefaultBranch),
		AutoDeployEnabled: a.AutoDeployEnabled,
		AutoDeployBranch:  deref(a.AutoDeployBranch),
		StoppedBy:         deref(a.StoppedBy),
		InsertedAt:        a.InsertedAt.Time,
		UpdatedAt:         a.UpdatedAt.Time,
		Settings: Settings{
			ContainerPort:   a.ContainerPort,
			HealthCheckType: a.HealthCheckType,
			HealthCheckPath: deref(a.HealthCheckPath),
			MemoryLimitMB:   a.MemoryLimitMB,
			CPULimit:        a.CPULimit,
			PidsLimit:       a.PidsLimit,
		},
	}
	if a.HealthCheckPort != nil {
		app.Settings.HealthCheckPort = *a.HealthCheckPort
	}
	if a.StoppedAt != nil {
		stopped := a.StoppedAt.Time
		app.StoppedAt = &stopped
	}
	return app
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
