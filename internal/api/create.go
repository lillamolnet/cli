package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

// Repository is a GitHub repository that one of the user's GitHub
// integrations reaches. A repository that two integrations reach is listed
// once for each.
type Repository struct {
	// ID is GitHub's id for the repository.
	ID            int64
	FullName      string
	CloneURL      string
	DefaultBranch string
	Private       bool
	IntegrationID string
}

type repositoryAttributes struct {
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	IntegrationID string `json:"integration_id"`
}

// Repositories returns the GitHub repositories that the user's GitHub
// integrations reach. An app can be created from any of them.
func (c *Client) Repositories(ctx context.Context) ([]Repository, error) {
	var doc document[[]resource[repositoryAttributes], struct{}]
	if err := c.get(ctx, []string{"repositories"}, nil, &doc); err != nil {
		return nil, err
	}

	repos := make([]Repository, 0, len(doc.Data))
	for _, r := range doc.Data {
		id, _ := strconv.ParseInt(r.ID, 10, 64)
		a := r.Attributes
		repos = append(repos, Repository{
			ID: id, FullName: a.FullName, CloneURL: a.CloneURL, DefaultBranch: a.DefaultBranch,
			Private: a.Private, IntegrationID: a.IntegrationID,
		})
	}
	return repos, nil
}

// CreateApp is an app to create. The platform chooses the servers, and takes
// the repository's address and default branch from GitHub.
type CreateApp struct {
	RepoID        int64
	IntegrationID string
	Name          string
	Subdomain     string
	Description   string
	// AutoDeployBranch turns auto deploy on for pushes to the branch.
	AutoDeployBranch string
}

// CreateApp creates an app. It is not deployed. A taken subdomain is an
// *Error with status 409, and a free one in Meta["suggested_subdomain"].
func (c *Client) CreateApp(ctx context.Context, a CreateApp) (App, error) {
	attributes := map[string]any{
		"repo_id":   a.RepoID,
		"name":      a.Name,
		"subdomain": a.Subdomain,
	}
	if a.IntegrationID != "" {
		attributes["integration_id"] = a.IntegrationID
	}
	if a.Description != "" {
		attributes["description"] = a.Description
	}
	if a.AutoDeployBranch != "" {
		attributes["auto_deploy_enabled"] = true
		attributes["auto_deploy_branch"] = a.AutoDeployBranch
	}

	body, err := json.Marshal(map[string]any{"data": map[string]any{"type": "app", "attributes": attributes}})
	if err != nil {
		return App{}, err
	}

	var doc document[resource[appAttributes], struct{}]
	if err := c.document(ctx, request{method: http.MethodPost, path: []string{"apps"}, body: body}, &doc); err != nil {
		return App{}, err
	}
	return toApp(doc.Data), nil
}
