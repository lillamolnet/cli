package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lillamolnet/cli/internal/api"
)

// The apps list has Bakery with the same id as the status fixture.
func statusServer(t *testing.T) string {
	t.Helper()
	return apiServer(t, map[string]string{
		"/api/v1/apps": "apps_for_status.json",
		"/api/v1/apps/fdf37127-5906-4c4c-9005-4da8cf355adb": "app_status.json",
	}, 404, `{"errors":[{"status":"404","code":"not_found","detail":"not found"}]}`)
}

func TestStatus(t *testing.T) {
	url := statusServer(t)

	for _, ref := range []string{"bakery", "fdf37127-5906-4c4c-9005-4da8cf355adb", "Bakery", "BAKERY"} {
		code, stdout, stderr := runWithAPI(t, url, "status", ref)
		if code != exitOK {
			t.Fatalf("%s: exit code = %d, stderr %q", ref, code, stderr)
		}

		for _, want := range []string{
			"Bakery (bakery)",
			"https://bakery.lillamolnet.se",
			"started",
			"failed, 2026-10-02 10:00 UTC, commit def5678, branch main",
			"mix compile failed",
			"deployed, 2026-10-01 10:03 UTC, commit abc1234",
			"Port:           4000\n",
			"Health check:   http /health\n",
			"Limits:         512 MB memory, 1 CPU, 256 processes\n",
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("%s: stdout has no %q:\n%s", ref, want, stdout)
			}
		}
	}
}

func TestStatusJSON(t *testing.T) {
	code, stdout, _ := runWithAPI(t, statusServer(t), "status", "bakery", "--json")
	if code != exitOK {
		t.Fatalf("exit code = %d", code)
	}

	var got struct {
		App          map[string]any `json:"app"`
		LatestBuild  map[string]any `json:"latest_build"`
		LatestDeploy map[string]any `json:"latest_deploy"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}

	if got.App["subdomain"] != "bakery" || got.App["url"] != "https://bakery.lillamolnet.se" {
		t.Errorf("app = %v", got.App)
	}

	settings := map[string]any{"container_port": 4000.0, "health_check_type": "http", "health_check_path": "/health",
		"health_check_port": nil, "memory_limit_mb": 512.0, "cpu_limit": 1.0, "pids_limit": 256.0}
	for key, want := range settings {
		if value, ok := got.App[key]; !ok || value != want {
			t.Errorf("app.%s = %v, want %v", key, value, want)
		}
	}

	wantBuild := map[string]any{
		"id":           "f712304f-d4c6-4c1d-be59-40a3cb90920d",
		"status":       "failed",
		"branch":       "main",
		"commit":       "def5678abc",
		"trigger":      "manual",
		"error_reason": "mix compile failed",
		"started_at":   "2026-10-02T10:00:00Z",
		"finished_at":  "2026-10-02T10:01:00Z",
	}
	if !jsonEqual(got.LatestBuild, wantBuild) {
		t.Errorf("latest_build = %v\nwant %v", got.LatestBuild, wantBuild)
	}

	if got.LatestDeploy["status"] != "deployed" || got.LatestDeploy["error_reason"] != nil {
		t.Errorf("latest_deploy = %v", got.LatestDeploy)
	}
	build, _ := got.LatestDeploy["build"].(map[string]any)
	if build["commit"] != "abc1234def" || build["status"] != "success" {
		t.Errorf("deployed build = %v", build)
	}
}

func TestStatusUnknownApp(t *testing.T) {
	code, stdout, stderr := runWithAPI(t, statusServer(t), "status", "nope")

	if code != exitNotFound {
		t.Errorf("exit code = %d, want %d", code, exitNotFound)
	}
	if stdout != "" || !strings.Contains(stderr, `no app "nope"`) {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
}

func TestStatusNeedsOneApp(t *testing.T) {
	for _, args := range [][]string{{"status"}, {"status", "a", "b"}} {
		code, _, _ := runWithAPI(t, statusServer(t), args...)
		if code != exitUsage {
			t.Errorf("%v: exit code = %d, want %d", args, code, exitUsage)
		}
	}
}

func TestStatusErrors(t *testing.T) {
	testAPIErrors(t, "status", "bakery")
}

func TestDescribeSettings(t *testing.T) {
	cases := map[string]api.Settings{
		"http /":        {ContainerPort: 4000, HealthCheckType: "http"},
		"http /healthz": {ContainerPort: 4000, HealthCheckType: "http", HealthCheckPath: "/healthz"},
		"tcp":           {ContainerPort: 5000, HealthCheckType: "tcp", HealthCheckPort: 5000},
		"tcp 5027":      {ContainerPort: 4000, HealthCheckType: "tcp", HealthCheckPort: 5027},
	}
	for want, s := range cases {
		if got := describeHealthCheck(s); got != want {
			t.Errorf("describeHealthCheck(%+v) = %q, want %q", s, got, want)
		}
	}

	limits := map[string]api.Settings{
		"512 MB memory, 1 CPU, 256 processes":    {MemoryLimitMB: 512, CPULimit: 1, PidsLimit: 256},
		"1024 MB memory, 0.5 CPU, 100 processes": {MemoryLimitMB: 1024, CPULimit: 0.5, PidsLimit: 100},
		"2048 MB memory, 2 CPUs, 512 processes":  {MemoryLimitMB: 2048, CPULimit: 2, PidsLimit: 512},
	}
	for want, s := range limits {
		if got := describeLimits(s); got != want {
			t.Errorf("describeLimits(%+v) = %q, want %q", s, got, want)
		}
	}
}
