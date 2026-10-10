package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWhoami(t *testing.T) {
	url := apiServer(t, map[string]string{"/api/v1/me": "me.json"}, 404, "")

	code, stdout, stderr := runWithAPI(t, url, "whoami")
	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}

	for _, want := range []string{"ada@example.com", "laptop", "read, deploy", "2027-01-07"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout has no %q:\n%s", want, stdout)
		}
	}
}

func TestWhoamiJSON(t *testing.T) {
	url := apiServer(t, map[string]string{"/api/v1/me": "me.json"}, 404, "")

	code, stdout, _ := runWithAPI(t, url, "whoami", "--json")
	if code != exitOK {
		t.Fatalf("exit code = %d", code)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}

	want := map[string]any{
		"id":    "2e3dd1de-6ec3-4aec-8c17-21a3d96ad63b",
		"email": "ada@example.com",
		"api_key": map[string]any{
			"name":       "laptop",
			"scopes":     []any{"read", "deploy"},
			"expires_at": "2027-01-07T20:10:44Z",
		},
	}
	if !jsonEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

func TestWhoamiErrors(t *testing.T) {
	testAPIErrors(t, "whoami")
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
