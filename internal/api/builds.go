package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// MaxBuildsPerPage is the most builds the API returns at once.
const MaxBuildsPerPage = 100

// Builds returns an app's builds, newest first, at most limit of them. An app
// that is not the user's has no builds.
func (c *Client) Builds(ctx context.Context, appID string, limit int) ([]Build, error) {
	if limit < 1 || limit > MaxBuildsPerPage {
		return nil, fmt.Errorf("limit must be between 1 and %d, got %d", MaxBuildsPerPage, limit)
	}
	query := url.Values{"page[limit]": {strconv.Itoa(limit)}}

	var doc document[[]resource[buildAttributes], struct{}]
	if err := c.get(ctx, []string{"apps", appID, "builds"}, query, &doc); err != nil {
		return nil, err
	}

	builds := make([]Build, 0, len(doc.Data))
	for _, r := range doc.Data {
		builds = append(builds, toBuild(r.ID, r.Attributes))
	}
	return builds, nil
}

// Build returns one of the user's builds.
func (c *Client) Build(ctx context.Context, id string) (Build, error) {
	var doc document[resource[buildAttributes], struct{}]
	if err := c.get(ctx, []string{"builds", id}, nil, &doc); err != nil {
		return Build{}, err
	}
	return toBuild(doc.Data.ID, doc.Data.Attributes), nil
}

// BuildLog returns a build's log. It is empty when the build has no log.
func (c *Client) BuildLog(ctx context.Context, id string) (string, error) {
	return c.getText(ctx, []string{"builds", id, "log"})
}

func toBuild(id string, a buildAttributes) Build {
	return Build{
		ID:          id,
		Status:      a.Status,
		Branch:      deref(a.Branch),
		Commit:      deref(a.RepoCommit),
		Trigger:     deref(a.TriggerType),
		ErrorReason: deref(a.ErrorReason),
		StartedAt:   a.StartedAt.Time,
		FinishedAt:  timePtr(a.FinishedAt),
	}
}

// decodeBuild decodes an included build.
func decodeBuild(included map[identifier]includedResource, ref identifier) (*Build, error) {
	r, ok := included[ref]
	if !ok {
		return nil, fmt.Errorf("unexpected response from the API: %s %s is not included", ref.Type, ref.ID)
	}

	var a buildAttributes
	if err := json.Unmarshal(r.Attributes, &a); err != nil {
		return nil, fmt.Errorf("unexpected response from the API: %w", err)
	}

	build := toBuild(r.ID, a)
	return &build, nil
}
