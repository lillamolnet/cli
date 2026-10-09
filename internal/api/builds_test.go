package api

import (
	"context"
	"testing"
	"time"
)

func TestBuildsFixturesFollowSpec(t *testing.T) {
	assertFollowsSpec(t, "/api/v1/apps/{app_id}/builds", readFixture(t, "builds.json"))
	assertFollowsSpec(t, "/api/v1/builds/{id}", readFixture(t, "build.json"))
}

func TestBuilds(t *testing.T) {
	client, req := server(t, 200, string(readFixture(t, "builds.json")))

	builds, err := client.Builds(context.Background(), "fdf37127-5906-4c4c-9005-4da8cf355adb", 20)
	if err != nil {
		t.Fatal(err)
	}

	if req.URL.Path != "/api/v1/apps/fdf37127-5906-4c4c-9005-4da8cf355adb/builds" {
		t.Errorf("path = %q", req.URL.Path)
	}
	if got := req.URL.Query().Get("page[limit]"); got != "20" {
		t.Errorf("page[limit] = %q", got)
	}

	if len(builds) != 2 {
		t.Fatalf("got %d builds", len(builds))
	}
	if b := builds[0]; b.Status != "failed" || b.Commit != "def5678abc" || b.ErrorReason != "mix compile failed" {
		t.Errorf("newest = %+v", b)
	}
	if b := builds[1]; b.Status != "success" || !b.StartedAt.Equal(time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("oldest = %+v", b)
	}
}

func TestBuildsLimit(t *testing.T) {
	client, _ := server(t, 200, `{"data":[]}`)

	for _, limit := range []int{0, -1, MaxBuildsPerPage + 1} {
		if _, err := client.Builds(context.Background(), "x", limit); err == nil {
			t.Errorf("limit %d should fail", limit)
		}
	}
}

func TestBuild(t *testing.T) {
	client, req := server(t, 200, string(readFixture(t, "build.json")))

	build, err := client.Build(context.Background(), "f712304f-d4c6-4c1d-be59-40a3cb90920d")
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/api/v1/builds/f712304f-d4c6-4c1d-be59-40a3cb90920d" {
		t.Errorf("path = %q", req.URL.Path)
	}
	if build.ID != "f712304f-d4c6-4c1d-be59-40a3cb90920d" || build.Status != "failed" || build.Trigger != "manual" {
		t.Errorf("build = %+v", build)
	}
}

func TestBuildLog(t *testing.T) {
	log := string(readFixture(t, "build.log"))
	client, req := server(t, 200, log)

	got, err := client.BuildLog(context.Background(), "f712304f-d4c6-4c1d-be59-40a3cb90920d")
	if err != nil {
		t.Fatal(err)
	}
	if got != log {
		t.Errorf("log = %q", got)
	}
	if req.URL.Path != "/api/v1/builds/f712304f-d4c6-4c1d-be59-40a3cb90920d/log" {
		t.Errorf("path = %q", req.URL.Path)
	}
	if accept := req.Header.Get("Accept"); accept != "text/plain" {
		t.Errorf("Accept = %q", accept)
	}
}
