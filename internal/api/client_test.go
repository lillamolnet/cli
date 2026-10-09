package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// server answers every request with status and body, and records the request.
func server(t *testing.T, status int, body string) (*Client, *http.Request) {
	t.Helper()
	var got http.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r.Clone(r.Context())
		w.Header().Set("Content-Type", mediaType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	client, err := New(srv.URL, "lillamolnet_test_key", "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	return client, &got
}

func TestRequestHeaders(t *testing.T) {
	client, req := server(t, 200, `{"data": []}`)

	var out document[[]resource[map[string]any], map[string]any]
	if err := client.get(context.Background(), []string{"apps"}, nil, &out); err != nil {
		t.Fatal(err)
	}

	if req.URL.Path != "/api/v1/apps" {
		t.Errorf("path = %q", req.URL.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer lillamolnet_test_key" {
		t.Errorf("Authorization = %q", got)
	}
	if got := req.Header.Get("Accept"); got != mediaType {
		t.Errorf("Accept = %q", got)
	}
	if got := req.Header.Get("User-Agent"); got != "lillamolnet-cli/1.2.3" {
		t.Errorf("User-Agent = %q", got)
	}
}

func TestErrorResponses(t *testing.T) {
	cases := []struct {
		status int
		body   string
		detail string
	}{
		{401, `{"errors":[{"status":"401","code":"unauthorized","detail":"A valid API key is required."}]}`, "A valid API key is required."},
		{403, `{"errors":[{"status":"403","code":"forbidden","detail":"forbidden"}],"jsonapi":{"version":"1.0"}}`, "forbidden"},
		{404, `{"errors":[{"status":"404","code":"no_route_found","detail":"no route found"}]}`, "no route found"},
		// Phoenix's own error body, not JSON:API.
		{500, `{"errors":{"detail":"Internal Server Error"}}`, ""},
		{502, `<html>Bad Gateway</html>`, ""},
	}

	for _, tc := range cases {
		client, _ := server(t, tc.status, tc.body)

		var out document[[]resource[map[string]any], map[string]any]
		err := client.get(context.Background(), []string{"apps"}, nil, &out)

		var apiErr *Error
		if !errors.As(err, &apiErr) {
			t.Fatalf("%d: err = %v, want *Error", tc.status, err)
		}
		if apiErr.Status != tc.status || apiErr.Detail != tc.detail {
			t.Errorf("%d: got %+v", tc.status, apiErr)
		}
		if !IsStatus(err, tc.status) {
			t.Errorf("%d: IsStatus is false", tc.status)
		}
	}
}

func TestConnectionError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	client, err := New(url, "key", "dev")
	if err != nil {
		t.Fatal(err)
	}

	var out document[[]resource[map[string]any], map[string]any]
	err = client.get(context.Background(), []string{"apps"}, nil, &out)

	var connErr *ConnectionError
	if !errors.As(err, &connErr) {
		t.Fatalf("err = %v, want *ConnectionError", err)
	}
}

func TestInvalidBaseURL(t *testing.T) {
	for _, u := range []string{"", "lillamolnet.se", "ftp://lillamolnet.se", "https://"} {
		if _, err := New(u, "key", "dev"); err == nil {
			t.Errorf("New(%q) should fail", u)
		}
	}
}
