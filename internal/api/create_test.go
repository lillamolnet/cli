package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateFixturesFollowSpec(t *testing.T) {
	assertFollowsSpec(t, "/api/v1/repositories", readFixture(t, "repositories.json"))
	assertFollowsSpec(t, "/api/v1/repositories", readFixture(t, "repositories_two_accounts.json"))
	assertFollowsSpecFor(t, http.MethodPost, "/api/v1/apps", 201, readFixture(t, "app_created.json"))
}

func TestRepositories(t *testing.T) {
	client, req := server(t, 200, string(readFixture(t, "repositories.json")))

	repos, err := client.Repositories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/api/v1/repositories" {
		t.Errorf("path = %q", req.URL.Path)
	}
	want := Repository{
		ID: 731, FullName: "example/Bakery", CloneURL: "https://github.com/example/Bakery.git",
		DefaultBranch: "main", Private: true, IntegrationID: "4b9f4d2e-2b8c-4c3e-9a51-0a7d1f6c2e11",
	}
	if len(repos) != 2 || repos[0] != want || repos[1].FullName != "example/site" || repos[1].Private {
		t.Errorf("repos = %+v", repos)
	}
}

func TestCreateApp(t *testing.T) {
	var method, path string
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sent)
		w.WriteHeader(201)
		_, _ = w.Write(readFixture(t, "app_created.json"))
	}))
	t.Cleanup(srv.Close)
	client, _ := New(srv.URL, "key", "dev")

	app, err := client.CreateApp(context.Background(), CreateApp{
		RepoID: 731, IntegrationID: "int", Name: "Bakery", Subdomain: "bakery", AutoDeployBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}

	if method != http.MethodPost || path != "/api/v1/apps" {
		t.Errorf("request = %s %s", method, path)
	}
	attrs := sent["data"].(map[string]any)["attributes"].(map[string]any)
	if attrs["repo_id"] != 731.0 || attrs["integration_id"] != "int" || attrs["name"] != "Bakery" ||
		attrs["subdomain"] != "bakery" || attrs["auto_deploy_enabled"] != true || attrs["auto_deploy_branch"] != "main" {
		t.Errorf("attributes = %v", attrs)
	}
	if _, ok := attrs["description"]; ok {
		t.Errorf("an empty description was sent: %v", attrs)
	}
	if app.Subdomain != "bakery" || app.RepoURL != "https://github.com/example/Bakery.git" {
		t.Errorf("app = %+v", app)
	}
}

func TestCreateAppConflict(t *testing.T) {
	client, _ := server(t, 409, string(readFixture(t, "app_create_conflict.json")))

	_, err := client.CreateApp(context.Background(), CreateApp{RepoID: 1, Name: "b", Subdomain: "bakery"})

	apiErr, ok := err.(*Error)
	if !ok || apiErr.Status != 409 || apiErr.Code != "subdomain_taken" || apiErr.Meta["suggested_subdomain"] != "bakery-2" {
		t.Errorf("err = %#v", err)
	}
}
