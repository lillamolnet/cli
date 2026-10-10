package api

import (
	"context"
	"net/http"
	"testing"
)

func TestPowerFixturesFollowSpec(t *testing.T) {
	assertFollowsSpecFor(t, http.MethodPost, "/api/v1/apps/{id}/stop", 200, readFixture(t, "app_stopped.json"))
	assertFollowsSpecFor(t, http.MethodPost, "/api/v1/apps/{id}/start", 200, readFixture(t, "app_started.json"))
}

func TestStopApp(t *testing.T) {
	client, req := server(t, 200, string(readFixture(t, "app_stopped.json")))

	app, result, err := client.StopApp(context.Background(), "fdf37127-5906-4c4c-9005-4da8cf355adb")
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != http.MethodPost || req.URL.Path != "/api/v1/apps/fdf37127-5906-4c4c-9005-4da8cf355adb/stop" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if result != PowerStopped || app.StoppedAt == nil || app.StoppedBy != "user" || app.URL == "" {
		t.Errorf("result = %q, app = %+v", result, app)
	}
}

func TestStartApp(t *testing.T) {
	client, req := server(t, 200, string(readFixture(t, "app_started.json")))

	app, result, err := client.StartApp(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/api/v1/apps/x/start" || result != PowerStarted || app.StoppedAt != nil {
		t.Errorf("path = %q, result = %q, app = %+v", req.URL.Path, result, app)
	}
}

func TestStartAppStoppedByPlatform(t *testing.T) {
	client, _ := server(t, 409, string(readFixture(t, "app_stopped_by_platform.json")))

	_, _, err := client.StartApp(context.Background(), "x")
	if apiErr, ok := err.(*Error); !ok || apiErr.Code != "stopped_by_platform" {
		t.Errorf("err = %v", err)
	}
}
