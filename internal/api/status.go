package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// Build is a build of an app.
type Build struct {
	ID          string
	Status      string
	Branch      string
	Commit      string
	Trigger     string
	ErrorReason string
	StartedAt   time.Time
	FinishedAt  *time.Time
}

// Deploy is a deploy of a build.
type Deploy struct {
	ID          string
	Status      string
	ErrorReason string
	StartedAt   time.Time
	FinishedAt  *time.Time
	// Build is the build that was deployed.
	Build *Build
}

// AppStatus is an app with its latest build and latest deploy. The latest
// deploy can belong to an older build than the latest, for example when the
// latest build failed.
type AppStatus struct {
	App          App
	LatestBuild  *Build
	LatestDeploy *Deploy
}

type buildAttributes struct {
	Status      string   `json:"status"`
	Branch      *string  `json:"branch"`
	RepoCommit  *string  `json:"repo_commit"`
	TriggerType *string  `json:"trigger_type"`
	ErrorReason *string  `json:"error_reason"`
	StartedAt   apiTime  `json:"started_at"`
	FinishedAt  *apiTime `json:"finished_at"`
}

type deployAttributes struct {
	Status      string   `json:"status"`
	ErrorReason *string  `json:"error_reason"`
	StartedAt   apiTime  `json:"started_at"`
	FinishedAt  *apiTime `json:"finished_at"`
}

type identifier struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type relationships map[string]struct {
	Data *identifier `json:"data"`
}

// related returns the identifier of a to-one relationship, or nil.
func (r relationships) related(name string) *identifier {
	if rel, ok := r[name]; ok {
		return rel.Data
	}
	return nil
}

type includedResource struct {
	identifier
	Attributes    json.RawMessage `json:"attributes"`
	Relationships relationships   `json:"relationships"`
}

type statusDocument struct {
	Data struct {
		resource[appAttributes]
		Relationships relationships `json:"relationships"`
	} `json:"data"`
	Included []includedResource `json:"included"`
}

// AppStatus returns the app with the id, with its latest build and deploy.
func (c *Client) AppStatus(ctx context.Context, id string) (AppStatus, error) {
	query := url.Values{"include": {"latest_build,latest_deploy,latest_deploy.build"}}

	var doc statusDocument
	if err := c.get(ctx, []string{"apps", id}, query, &doc); err != nil {
		return AppStatus{}, err
	}

	included := make(map[identifier]includedResource, len(doc.Included))
	for _, r := range doc.Included {
		included[r.identifier] = r
	}

	status := AppStatus{App: toApp(doc.Data.resource)}

	if ref := doc.Data.Relationships.related("latest_build"); ref != nil {
		build, err := decodeBuild(included, *ref)
		if err != nil {
			return AppStatus{}, err
		}
		status.LatestBuild = build
	}

	if ref := doc.Data.Relationships.related("latest_deploy"); ref != nil {
		r, ok := included[*ref]
		if !ok {
			return AppStatus{}, fmt.Errorf("unexpected response from the API: %s %s is not included", ref.Type, ref.ID)
		}

		var a deployAttributes
		if err := json.Unmarshal(r.Attributes, &a); err != nil {
			return AppStatus{}, fmt.Errorf("unexpected response from the API: %w", err)
		}

		deploy := &Deploy{
			ID:          r.ID,
			Status:      a.Status,
			ErrorReason: deref(a.ErrorReason),
			StartedAt:   a.StartedAt.Time,
			FinishedAt:  timePtr(a.FinishedAt),
		}
		if buildRef := r.Relationships.related("build"); buildRef != nil {
			build, err := decodeBuild(included, *buildRef)
			if err != nil {
				return AppStatus{}, err
			}
			deploy.Build = build
		}
		status.LatestDeploy = deploy
	}

	return status, nil
}

func timePtr(t *apiTime) *time.Time {
	if t == nil {
		return nil
	}
	v := t.Time
	return &v
}
