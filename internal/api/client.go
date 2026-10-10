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
		// No client-wide timeout: each request sets its own, since waiting for a
		// deploy takes longer than an ordinary call.
		http: &http.Client{},
	}, nil
}

// Error is an error response from the API.
type Error struct {
	Status int
	Code   string
	Detail string
	// Meta is the error's meta object, such as the running build's id when a
	// deploy is refused with already_building.
	Meta map[string]any
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
		Code   string         `json:"code"`
		Detail string         `json:"detail"`
		Meta   map[string]any `json:"meta"`
	} `json:"errors"`
}

// Response size limits. Build logs can be several MB.
const (
	maxDocumentSize = 10 << 20
	maxTextSize     = 100 << 20
)

// defaultTimeout is how long an ordinary request may take.
const defaultTimeout = 30 * time.Second

// request is one call to the API.
type request struct {
	method  string
	path    []string
	query   url.Values
	accept  string
	limit   int64
	timeout time.Duration
}

// get fetches path, relative to /api/v1, with the query, and decodes the
// document into out. JoinPath escapes the path elements.
func (c *Client) get(ctx context.Context, path []string, query url.Values, out any) error {
	return c.document(ctx, request{method: http.MethodGet, path: path, query: query}, out)
}

// document makes the request and decodes the JSON:API document into out.
func (c *Client) document(ctx context.Context, r request, out any) error {
	r.accept, r.limit = mediaType, maxDocumentSize
	body, err := c.fetch(ctx, r)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("unexpected response from the API: %w", err)
	}
	return nil
}

// getText fetches path as plain text.
func (c *Client) getText(ctx context.Context, path []string) (string, error) {
	body, err := c.fetch(ctx, request{method: http.MethodGet, path: path, accept: "text/plain", limit: maxTextSize})
	return string(body), err
}

func (c *Client) fetch(ctx context.Context, r request) ([]byte, error) {
	u := c.baseURL.JoinPath(append([]string{"api", "v1"}, r.path...)...)
	u.RawQuery = r.query.Encode()

	timeout := r.timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, r.method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", r.accept)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &ConnectionError{Err: err}
	}
	defer resp.Body.Close()

	// One byte more than the limit, so that a larger response is an error
	// rather than silently cut off.
	body, err := io.ReadAll(io.LimitReader(resp.Body, r.limit+1))
	if err != nil {
		return nil, &ConnectionError{Err: err}
	}
	if int64(len(body)) > r.limit {
		return nil, fmt.Errorf("the response from the API is larger than %d MB", r.limit>>20)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, decodeError(resp.StatusCode, body)
	}
	return body, nil
}

func decodeError(status int, body []byte) error {
	apiErr := &Error{Status: status}

	var doc errorDocument
	if json.Unmarshal(body, &doc) == nil && len(doc.Errors) > 0 {
		apiErr.Code = doc.Errors[0].Code
		apiErr.Detail = doc.Errors[0].Detail
		apiErr.Meta = doc.Errors[0].Meta
	}
	return apiErr
}

// IsStatus reports whether err is an API error with the HTTP status.
func IsStatus(err error, status int) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Status == status
}
