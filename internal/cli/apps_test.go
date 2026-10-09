package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApps(t *testing.T) {
	url := apiServer(t, map[string]string{"/api/v1/apps": "apps.json"}, 404, "")

	code, stdout, stderr := runWithAPI(t, url, "apps")
	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 {
		t.Fatalf("want a header and two apps:\n%s", stdout)
	}
	for _, want := range []string{"Bakery", "bakery", "started", "main", "https://github.com/example/bakery"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("first app has no %q: %q", want, lines[1])
		}
	}
	for _, want := range []string{"Old shop", "stopped", "off"} {
		if !strings.Contains(lines[2], want) {
			t.Errorf("second app has no %q: %q", want, lines[2])
		}
	}
}

func TestAppsJSON(t *testing.T) {
	url := apiServer(t, map[string]string{"/api/v1/apps": "apps.json"}, 404, "")

	code, stdout, _ := runWithAPI(t, url, "apps", "--json")
	if code != exitOK {
		t.Fatalf("exit code = %d", code)
	}

	var got []map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("not a JSON array: %v\n%s", err, stdout)
	}
	if len(got) != 2 {
		t.Fatalf("got %d apps", len(got))
	}

	want := map[string]any{
		"id":                  "c4c0ea51-0000-4000-8000-000000000002",
		"name":                "Old shop",
		"subdomain":           "old-shop",
		"description":         nil,
		"repo_url":            "https://github.com/example/shop",
		"default_branch":      nil,
		"auto_deploy_enabled": false,
		"auto_deploy_branch":  nil,
		"stopped":             true,
		"stopped_at":          "2026-10-05T12:30:00Z",
		"stopped_by":          "user",
		"created_at":          "2026-09-01T10:00:00Z",
		"updated_at":          "2026-10-05T12:30:00Z",
	}
	if !jsonEqual(got[1], want) {
		t.Errorf("got  %v\nwant %v", got[1], want)
	}
	if got[0]["stopped"] != false || got[0]["stopped_at"] != nil {
		t.Errorf("first app should be started: %v", got[0])
	}
}

func TestAppsEmpty(t *testing.T) {
	url := apiServer(t, nil, 200, `{"data":[],"jsonapi":{"version":"1.0"},"links":{},"meta":{}}`)

	code, stdout, _ := runWithAPI(t, url, "apps")
	if code != exitOK || !strings.Contains(stdout, "No apps yet.") {
		t.Errorf("exit code = %d, stdout = %q", code, stdout)
	}

	code, stdout, _ = runWithAPI(t, url, "apps", "--json")
	if code != exitOK || strings.TrimSpace(stdout) != "[]" {
		t.Errorf("exit code = %d, stdout = %q", code, stdout)
	}
}

func TestAppsErrors(t *testing.T) {
	testAPIErrors(t, "apps")
}
