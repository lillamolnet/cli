package api

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

const queuedBuildID = "560b8ba0-8027-449f-b695-d5a4c13077bc"

func TestDeployFixturesFollowSpec(t *testing.T) {
	assertFollowsSpecFor(t, http.MethodPost, "/api/v1/apps/{id}/deploys", 202, readFixture(t, "deploy_started.json"))
	assertFollowsSpecFor(t, http.MethodGet, "/api/v1/builds/{id}/wait", 200, readFixture(t, "wait_queued.json"))
	assertFollowsSpecFor(t, http.MethodGet, "/api/v1/builds/{id}/wait", 200, readFixture(t, "wait_deployed.json"))
}

func TestStartDeploy(t *testing.T) {
	client, req := server(t, 202, string(readFixture(t, "deploy_started.json")))

	build, err := client.StartDeploy(context.Background(), "fdf37127-5906-4c4c-9005-4da8cf355adb")
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

func TestStartDeployAlreadyBuilding(t *testing.T) {
	client, _ := server(t, 409, string(readFixture(t, "deploy_conflict.json")))

	_, err := client.StartDeploy(context.Background(), "x")

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

	result, err := client.Wait(context.Background(), queuedBuildID, 50*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if req.URL.Path != "/api/v1/builds/"+queuedBuildID+"/wait" || req.URL.Query().Get("timeout") != "50" {
		t.Errorf("request = %s?%s", req.URL.Path, req.URL.RawQuery)
	}
	if result.Done || result.Result != ResultQueued || result.Deploy != nil || result.Build.Status != "queued" {
		t.Errorf("result = %+v", result)
	}
}

func TestWaitDeployed(t *testing.T) {
	client, _ := server(t, 200, string(readFixture(t, "wait_deployed.json")))

	result, err := client.Wait(context.Background(), queuedBuildID, 0)
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
		if _, err := client.Wait(context.Background(), "x", timeout); err == nil {
			t.Errorf("timeout %s should fail", timeout)
		}
	}
}
