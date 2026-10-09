package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const failedBuildID = "f712304f-d4c6-4c1d-be59-40a3cb90920d"

func buildsServer(t *testing.T) string {
	t.Helper()
	return apiServer(t, map[string]string{
		"/api/v1/apps": "apps_for_status.json",
		"/api/v1/apps/fdf37127-5906-4c4c-9005-4da8cf355adb/builds": "builds.json",
		"/api/v1/builds/" + failedBuildID + "/log":                 "build.log",
	}, 404, `{"errors":[{"status":"404","code":"not_found","detail":"No build record found"}]}`)
}

func TestBuilds(t *testing.T) {
	code, stdout, stderr := runWithAPI(t, buildsServer(t), "builds", "bakery")
	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 {
		t.Fatalf("want a header and two builds:\n%s", stdout)
	}
	for _, want := range []string{"2026-10-02 10:00 UTC", "failed", "def5678", "main", "manual", failedBuildID} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("newest build has no %q: %q", want, lines[1])
		}
	}
}

func TestBuildsJSON(t *testing.T) {
	code, stdout, _ := runWithAPI(t, buildsServer(t), "builds", "bakery", "--json")
	if code != exitOK {
		t.Fatalf("exit code = %d", code)
	}

	var got []map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("not a JSON array: %v\n%s", err, stdout)
	}
	if len(got) != 2 || got[0]["id"] != failedBuildID || got[0]["commit"] != "def5678abc" ||
		got[0]["error_reason"] != "mix compile failed" {
		t.Errorf("got %v", got)
	}
}

func TestBuildsLimitFlag(t *testing.T) {
	for _, limit := range []string{"0", "101", "x"} {
		code, _, _ := runWithAPI(t, buildsServer(t), "builds", "bakery", "--limit", limit)
		if code != exitUsage {
			t.Errorf("--limit %s: exit code = %d, want %d", limit, code, exitUsage)
		}
	}
}

func TestBuildsUnknownApp(t *testing.T) {
	code, _, _ := runWithAPI(t, buildsServer(t), "builds", "nope")
	if code != exitNotFound {
		t.Errorf("exit code = %d, want %d", code, exitNotFound)
	}
}

func TestBuildsErrors(t *testing.T) {
	testAPIErrors(t, "builds", "bakery")
}

func TestBuildLogs(t *testing.T) {
	want, err := os.ReadFile("../api/testdata/build.log")
	if err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runWithAPI(t, buildsServer(t), "build-logs", failedBuildID)
	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	if stdout != string(want) {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestBuildLogsJSON(t *testing.T) {
	code, stdout, _ := runWithAPI(t, buildsServer(t), "build-logs", failedBuildID, "--json")
	if code != exitOK {
		t.Fatalf("exit code = %d", code)
	}

	var got buildLogJSON
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}
	if got.ID != failedBuildID || !strings.Contains(got.Log, "mix compile failed") {
		t.Errorf("got %+v", got)
	}
}

func TestBuildLogsUnknownBuild(t *testing.T) {
	code, stdout, stderr := runWithAPI(t, buildsServer(t), "build-logs", "00000000-0000-0000-0000-000000000000")
	if code != exitNotFound {
		t.Errorf("exit code = %d, want %d", code, exitNotFound)
	}
	if stdout != "" || !strings.Contains(stderr, "No build record found") {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
}

func TestBuildLogsErrors(t *testing.T) {
	testAPIErrors(t, "build-logs", failedBuildID)
}
