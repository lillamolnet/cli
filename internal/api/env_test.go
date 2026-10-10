package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnvFixturesFollowSpec(t *testing.T) {
	assertFollowsSpecFor(t, http.MethodGet, "/api/v1/apps/{app_id}/env", 200, readFixture(t, "env_list.json"))
	assertFollowsSpecFor(t, http.MethodPut, "/api/v1/apps/{app_id}/env/{key}", 201, readFixture(t, "env_created.json"))
	assertFollowsSpecFor(t, http.MethodPut, "/api/v1/apps/{app_id}/env/{key}", 200, readFixture(t, "env_rotated.json"))
}

func TestEnvVars(t *testing.T) {
	client, req := server(t, 200, string(readFixture(t, "env_list.json")))

	vars, err := client.EnvVars(context.Background(), "fdf37127-5906-4c4c-9005-4da8cf355adb")
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/api/v1/apps/fdf37127-5906-4c4c-9005-4da8cf355adb/env" {
		t.Errorf("path = %q", req.URL.Path)
	}
	if len(vars) != 2 || vars[0].Key != "DATABASE_URL" || !vars[0].Secret || vars[0].Scope != "runtime" ||
		vars[1].Key != "PHX_HOST" || vars[1].Secret {
		t.Errorf("vars = %+v", vars)
	}
}

func TestSetEnvVar(t *testing.T) {
	var method, path, contentType string
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, contentType = r.Method, r.URL.Path, r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sent)
		w.WriteHeader(201)
		_, _ = w.Write(readFixture(t, "env_created.json"))
	}))
	t.Cleanup(srv.Close)
	client, _ := New(srv.URL, "key", "dev")

	secret := true
	v, result, err := client.SetEnvVar(context.Background(), "app", "SECRET_KEY_BASE", SetEnvVar{Value: "s3cret", Scope: "runtime", Secret: &secret})
	if err != nil {
		t.Fatal(err)
	}

	if method != http.MethodPut || path != "/api/v1/apps/app/env/SECRET_KEY_BASE" || contentType != mediaType {
		t.Errorf("request = %s %s (%s)", method, path, contentType)
	}
	attrs := sent["data"].(map[string]any)["attributes"].(map[string]any)
	if attrs["value"] != "s3cret" || attrs["scope"] != "runtime" || attrs["is_secret"] != true {
		t.Errorf("attributes = %v", attrs)
	}
	if result != EnvCreated || v.Key != "SECRET_KEY_BASE" || !v.Secret {
		t.Errorf("result = %q, var = %+v", result, v)
	}
}

func TestSetEnvVarOnlySendsWhatIsGiven(t *testing.T) {
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sent = string(body)
		_, _ = w.Write(readFixture(t, "env_rotated.json"))
	}))
	t.Cleanup(srv.Close)
	client, _ := New(srv.URL, "key", "dev")

	_, result, err := client.SetEnvVar(context.Background(), "app", "DATABASE_URL", SetEnvVar{Value: "v"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sent, "scope") || strings.Contains(sent, "is_secret") || result != EnvRotated {
		t.Errorf("sent = %s, result = %q", sent, result)
	}
}

func TestSetEnvVarErrors(t *testing.T) {
	for _, tc := range []struct {
		status  int
		fixture string
		code    string
	}{
		{409, "env_conflict.json", "secret_is_secret"},
		{422, "env_invalid.json", "invalid_attribute"},
	} {
		client, _ := server(t, tc.status, string(readFixture(t, tc.fixture)))

		_, _, err := client.SetEnvVar(context.Background(), "app", "X", SetEnvVar{Value: "v"})

		apiErr, ok := err.(*Error)
		if !ok || apiErr.Status != tc.status || apiErr.Code != tc.code {
			t.Errorf("%d: err = %v", tc.status, err)
		}
	}
}

func TestUnsetEnvVar(t *testing.T) {
	client, req := server(t, 204, "")

	if err := client.UnsetEnvVar(context.Background(), "app", "PHX_HOST"); err != nil {
		t.Fatal(err)
	}
	if req.Method != http.MethodDelete || req.URL.Path != "/api/v1/apps/app/env/PHX_HOST" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}
