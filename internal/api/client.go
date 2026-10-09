// Package api is a small client for the Lilla Molnet API at /api/v1.
//
// The API speaks JSON:API. This package decodes it into plain Go types, so
// that the rest of the CLI never sees the JSON:API envelope.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the public API.
const DefaultBaseURL = "https://lillamolnet.se"

const mediaType = "application/vnd.api+json"

// Client calls /api/v1 with a personal API key.
type Client struct {
	baseURL   *url.URL
	token     string
	userAgent string
	http      *http.Client
}

// New returns a client for baseURL, such as https://lillamolnet.se.
func New(baseURL, token, version string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("invalid API URL %q: it must look like https://lillamolnet.se", baseURL)
	}

	return &Client{
		baseURL:   u,
		token:     token,
		userAgent: "lillamolnet-cli/" + version,
		http:      &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Error is an error response from the API.
type Error struct {
	Status int
	Code   string
	Detail string
}

func (e *Error) Error() string {
	switch {
	case e.Detail != "":
		return fmt.Sprintf("%s (HTTP %d)", e.Detail, e.Status)
	case e.Code != "":
		return fmt.Sprintf("%s (HTTP %d)", e.Code, e.Status)
	default:
		return fmt.Sprintf("HTTP %d", e.Status)
	}
}

// ConnectionError means the API could not be reached.
type ConnectionError struct{ Err error }

func (e *ConnectionError) Error() string { return "could not reach the API: " + e.Err.Error() }
func (e *ConnectionError) Unwrap() error { return e.Err }

// resource is a JSON:API resource object.
type resource[A any] struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Attributes A      `json:"attributes"`
}

// document is a JSON:API top-level document.
type document[D any, M any] struct {
	Data D `json:"data"`
	Meta M `json:"meta"`
}

// errorDocument is a JSON:API error document. Plain {"error": ...} bodies, for
// example from a proxy, decode to no errors.
type errorDocument struct {
	Errors []struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	} `json:"errors"`
}

// get fetches path, relative to /api/v1, with the query, and decodes the
// document into out. JoinPath escapes the path elements.
func (c *Client) get(ctx context.Context, path []string, query url.Values, out any) error {
	u := c.baseURL.JoinPath(append([]string{"api", "v1"}, path...)...)
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", mediaType)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return &ConnectionError{Err: err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return &ConnectionError{Err: err}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return decodeError(resp.StatusCode, body)
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("unexpected response from the API: %w", err)
	}
	return nil
}

func decodeError(status int, body []byte) error {
	apiErr := &Error{Status: status}

	var doc errorDocument
	if json.Unmarshal(body, &doc) == nil && len(doc.Errors) > 0 {
		apiErr.Code = doc.Errors[0].Code
		apiErr.Detail = doc.Errors[0].Detail
	}
	return apiErr
}

// IsStatus reports whether err is an API error with the HTTP status.
func IsStatus(err error, status int) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Status == status
}
