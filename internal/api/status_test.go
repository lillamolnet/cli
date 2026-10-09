package api

import (
	"context"
	"testing"
	"time"
)

func TestAppStatusFixtureFollowsSpec(t *testing.T) {
	assertFollowsSpec(t, "/api/v1/apps/{id}", readFixture(t, "app_status.json"))
}

func TestAppStatus(t *testing.T) {
	client, req := server(t, 200, string(readFixture(t, "app_status.json")))

	status, err := client.AppStatus(context.Background(), "fdf37127-5906-4c4c-9005-4da8cf355adb")
	if err != nil {
		t.Fatal(err)
	}

	if req.URL.Path != "/api/v1/apps/fdf37127-5906-4c4c-9005-4da8cf355adb" {
		t.Errorf("path = %q", req.URL.Path)
	}
	if got := req.URL.Query().Get("include"); got != "latest_build,latest_deploy,latest_deploy.build" {
		t.Errorf("include = %q", got)
	}

	if status.App.Name != "Bakery" || status.App.URL != "https://bakery.lillamolnet.se" {
		t.Errorf("app = %+v", status.App)
	}

	build := status.LatestBuild
	if build == nil || build.Status != "failed" || build.Commit != "def5678abc" ||
		build.ErrorReason != "mix compile failed" || build.Trigger != "manual" {
		t.Fatalf("latest build = %+v", build)
	}
	if !build.StartedAt.Equal(time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("StartedAt = %v", build.StartedAt)
	}

	deploy := status.LatestDeploy
	if deploy == nil || deploy.Status != "deployed" || deploy.FinishedAt == nil {
		t.Fatalf("latest deploy = %+v", deploy)
	}
	if deploy.Build == nil || deploy.Build.Commit != "abc1234def" || deploy.Build.Status != "success" {
		t.Errorf("deployed build = %+v", deploy.Build)
	}
}

func TestAppStatusWithoutBuilds(t *testing.T) {
	body := `{"data":{"id":"1","type":"app","attributes":{"name":"New","subdomain":"new",
		"inserted_at":"2026-10-09T10:00:00Z","updated_at":"2026-10-09T10:00:00Z"},
		"relationships":{"latest_build":{"data":null},"latest_deploy":{"data":null}}},"included":[]}`
	client, _ := server(t, 200, body)

	status, err := client.AppStatus(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if status.LatestBuild != nil || status.LatestDeploy != nil {
		t.Errorf("status = %+v", status)
	}
}
