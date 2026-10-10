package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// MaxWait is the longest one wait request may take on the server.
const MaxWait = 50 * time.Second

// The results of a wait. The first three mean that it is still going on.
const (
	ResultQueued       = "queued"
	ResultBuilding     = "building"
	ResultDeploying    = "deploying"
	ResultDeployed     = "deployed"
	ResultBuildFailed  = "build_failed"
	ResultDeployFailed = "deploy_failed"
)

// WaitResult is where a build and its deploy are.
type WaitResult struct {
	Build Build
	// Deploy is the build's latest deploy, or nil before there is one.
	Deploy *Deploy
	Done   bool
	Result string
}

// StartDeploy starts a build and a deploy of the app, and returns the queued
// build. When a build is already queued or running, the error is an
// *AlreadyBuildingError.
func (c *Client) StartDeploy(ctx context.Context, appID string) (Build, error) {
	var doc document[resource[buildAttributes], struct{}]
	err := c.document(ctx, request{method: http.MethodPost, path: []string{"apps", appID, "deploys"}}, &doc)

	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Code == "already_building" {
		buildID, _ := apiErr.Meta["build_id"].(string)
		return Build{}, &AlreadyBuildingError{BuildID: buildID, Err: apiErr}
	}
	if err != nil {
		return Build{}, err
	}
	return toBuild(doc.Data.ID, doc.Data.Attributes), nil
}

// AlreadyBuildingError means that a build of the app is already queued or
// running, so no new one was started.
type AlreadyBuildingError struct {
	BuildID string
	Err     *Error
}

func (e *AlreadyBuildingError) Error() string {
	return fmt.Sprintf("a build of the app is already queued or running: %s", e.BuildID)
}

func (e *AlreadyBuildingError) Unwrap() error { return e.Err }

type waitDocument struct {
	Data     resource[buildAttributes] `json:"data"`
	Included []includedResource        `json:"included"`
	Meta     struct {
		Done   bool   `json:"done"`
		Result string `json:"result"`
	} `json:"meta"`
}

// Wait waits for the build and its deploy to finish, for at most timeout on
// the server, and returns where they are. Call it again until Done.
func (c *Client) Wait(ctx context.Context, buildID string, timeout time.Duration) (WaitResult, error) {
	if timeout < 0 || timeout > MaxWait {
		return WaitResult{}, fmt.Errorf("timeout must be between 0 and %s, got %s", MaxWait, timeout)
	}

	query := url.Values{"timeout": {strconv.Itoa(int(timeout.Seconds()))}}
	r := request{
		method: http.MethodGet,
		path:   []string{"builds", buildID, "wait"},
		query:  query,
		// The server answers within the timeout. The rest is for the network.
		timeout: timeout + defaultTimeout,
	}

	var doc waitDocument
	if err := c.document(ctx, r, &doc); err != nil {
		return WaitResult{}, err
	}

	result := WaitResult{
		Build:  toBuild(doc.Data.ID, doc.Data.Attributes),
		Done:   doc.Meta.Done,
		Result: doc.Meta.Result,
	}

	for _, inc := range doc.Included {
		if inc.Type != "deploy" {
			continue
		}
		var a deployAttributes
		if err := json.Unmarshal(inc.Attributes, &a); err != nil {
			return WaitResult{}, fmt.Errorf("unexpected response from the API: %w", err)
		}
		result.Deploy = &Deploy{
			ID:          inc.ID,
			Status:      a.Status,
			ErrorReason: deref(a.ErrorReason),
			StartedAt:   a.StartedAt.Time,
			FinishedAt:  timePtr(a.FinishedAt),
		}
	}

	return result, nil
}
