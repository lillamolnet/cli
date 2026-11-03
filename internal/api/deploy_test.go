package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const queuedBuildID = "560b8ba0-8027-449f-b695-d5a4c13077bc"

func TestDeployFixturesFollowSpec(t *testing.T) {
	assertFollowsSpecFor(t, http.MethodPost, "/api/v1/apps/{id}/deploys", 202, readFixture(t, "deploy_started.json"))
	assertFollowsSpecFor(t, http.MethodPost, "/api/v1/apps/{id}/deploys", 202, readFixture(t, "deploy_started_branch.json"))
	assertFollowsSpecFor(t, http.MethodGet, "/api/v1/builds/{id}/wait", 200, readFixture(t, "wait_queued.json"))
	assertFollowsSpecFor(t, http.MethodGet, "/api/v1/builds/{id}/wait", 200, readFixture(t, "wait_deployed.json"))
}

func TestStartDeploy(t *testing.T) {
	client, req := server(t, 202, string(readFixture(t, "deploy_started.json")))

	build, err := client.StartDeploy(context.Background(), "fdf37127-5906-4c4c-9005-4da8cf355adb", "")
	if err != nil {
		t.Fatal(err)
	}

	if req.Method != http.MethodPost || req.URL.Path != "/api/v1/apps/fdf37127-5906-4c4c-9005-4da8cf355adb/deploys" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if build.ID != queuedBuildID || build.Status != "queued" || build.Trigger != "manual" {
		t.Errorf("build = %+v", build)
	}
}

func TestStartDeploySendsNoBodyWithoutBranch(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(202)
		_, _ = w.Write(readFixture(t, "deploy_started.json"))
	}))
	t.Cleanup(srv.Close)
	client, _ := New(srv.URL, "key", "dev")

	if _, err := client.StartDeploy(context.Background(), "x", ""); err != nil {
		t.Fatal(err)
	}
	if len(body) != 0 {
		t.Errorf("body = %s, want none", body)
	}
}

func TestStartDeployWithBranch(t *testing.T) {
	var contentType string
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sent)
		w.WriteHeader(202)
		_, _ = w.Write(readFixture(t, "deploy_started_branch.json"))
	}))
	t.Cleanup(srv.Close)
	client, _ := New(srv.URL, "key", "dev")

	build, err := client.StartDeploy(context.Background(), "x", "feature/new-menu")
	if err != nil {
		t.Fatal(err)
	}

	if contentType != mediaType {
		t.Errorf("Content-Type = %q", contentType)
	}
	attrs := sent["data"].(map[string]any)["attributes"].(map[string]any)
	if attrs["branch"] != "feature/new-menu" {
		t.Errorf("attributes = %v", attrs)
	}
	if build.Branch != "feature/new-menu" {
		t.Errorf("build = %+v", build)
	}
}

func TestStartDeployAlreadyBuilding(t *testing.T) {
	client, _ := server(t, 409, string(readFixture(t, "deploy_conflict.json")))

	_, err := client.StartDeploy(context.Background(), "x", "")

	var already *AlreadyBuildingError
	if !errors.As(err, &already) || already.BuildID != queuedBuildID {
		t.Fatalf("err = %v, want *AlreadyBuildingError with the build id", err)
	}
	if !IsStatus(err, 409) {
		t.Error("IsStatus(409) should hold")
	}
}

func TestWaitQueued(t *testing.T) {
	client, req := server(t, 200, string(readFixture(t, "wait_queued.json")))

	result, err := client.Wait(context.Background(), queuedBuildID, 50*time.Second, ResultQueued)
	if err != nil {
		t.Fatal(err)
	}

	if req.URL.Path != "/api/v1/builds/"+queuedBuildID+"/wait" || req.URL.Query().Get("timeout") != "50" || req.URL.Query().Get("since") != "queued" {
		t.Errorf("request = %s?%s", req.URL.Path, req.URL.RawQuery)
	}
	if result.Done || result.Result != ResultQueued || result.Deploy != nil || result.Build.Status != "queued" {
		t.Errorf("result = %+v", result)
	}
}

func TestWaitDeployed(t *testing.T) {
	client, _ := server(t, 200, string(readFixture(t, "wait_deployed.json")))

	result, err := client.Wait(context.Background(), queuedBuildID, 0, "")
	if err != nil {
		t.Fatal(err)
	}

	if !result.Done || result.Result != ResultDeployed {
		t.Errorf("result = %+v", result)
	}
	if result.Build.Commit != "def5678abc" || result.Deploy == nil || result.Deploy.Status != "deployed" {
		t.Errorf("build = %+v, deploy = %+v", result.Build, result.Deploy)
	}
}

func TestWaitTimeout(t *testing.T) {
	client, _ := server(t, 200, "{}")

	for _, timeout := range []time.Duration{-time.Second, MaxWait + time.Second} {
		if _, err := client.Wait(context.Background(), "x", timeout, ""); err == nil {
			t.Errorf("timeout %s should fail", timeout)
		}
	}
}
