package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func decodeErrorJSON(t *testing.T, stdout string) errorBody {
	t.Helper()
	var doc errorJSON
	dec := json.NewDecoder(strings.NewReader(stdout))
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("stdout is not a JSON error: %v\n%s", err, stdout)
	}
	if dec.More() {
		t.Errorf("stdout has more than one JSON value:\n%s", stdout)
	}
	return doc.Error
}

func TestErrorsAreJSONWithJSON(t *testing.T) {
	notFound := `{"errors":[{"status":"404","code":"not_found","detail":"No build record found"}]}`

	cases := []struct {
		name   string
		args   []string
		status int
		body   string
		code   string
		exit   int
		http   int
	}{
		{"usage before the flags are parsed", []string{"nope", "--json"}, 0, "", "usage", exitUsage, 0},
		{"unknown flag", []string{"apps", "--nope", "--json"}, 0, "", "usage", exitUsage, 0},
		{"not found", []string{"build-logs", queuedBuildID, "--json"}, 404, notFound, "not_found", exitNotFound, 404},
		{"invalid key", []string{"apps", "--json"}, 401, `{"errors":[{"status":"401","code":"unauthorized"}]}`, "auth", exitAuth, 401},
	}

	for _, tc := range cases {
		url := apiServer(t, nil, tc.status, tc.body)
		code, stdout, stderr := runWithAPI(t, url, tc.args...)

		if code != tc.exit {
			t.Errorf("%s: exit code = %d, want %d", tc.name, code, tc.exit)
		}
		got := decodeErrorJSON(t, stdout)
		if got.Code != tc.code || got.ExitCode != tc.exit || got.HTTPStatus != tc.http || got.Message == "" {
			t.Errorf("%s: error = %+v", tc.name, got)
		}
		// The text is still on stderr, for people.
		if !strings.HasPrefix(stderr, "Error: ") {
			t.Errorf("%s: stderr = %q", tc.name, stderr)
		}
	}
}

func TestForbiddenJSONNamesTheScopes(t *testing.T) {
	url := apiServer(t, nil, 403, `{"errors":[{"status":"403","code":"forbidden","detail":"forbidden"}]}`)

	code, stdout, _ := runWithAPI(t, url, "deploy", "bakery", "--json")

	got := decodeErrorJSON(t, stdout)
	if code != exitForbidden || got.Code != "forbidden" || strings.Join(got.RequiredScopes, ",") != "read,deploy" {
		t.Errorf("exit code = %d, error = %+v", code, got)
	}
}

// Without --json, stdout stays empty when a command fails.
func TestErrorsWithoutJSONLeaveStdoutEmpty(t *testing.T) {
	url := apiServer(t, nil, 404, `{"errors":[{"status":"404","code":"not_found"}]}`)

	_, stdout, _ := runWithAPI(t, url, "build-logs", queuedBuildID)
	if stdout != "" {
		t.Errorf("stdout = %q", stdout)
	}
}

// deploy --wait --json prints its result, which says that the build failed,
// and no error object after it.
func TestDeployFailureJSONIsOneValue(t *testing.T) {
	deployed := fixture(t, "wait_deployed.json")
	url := deployServer(t, 202, []waitAnswer{{200, withResult(t, deployed, "build_failed", "failed")}})

	code, stdout, _ := runWithAPI(t, url, "deploy", "bakery", "--wait", "--json")
	if code != exitBuildFailed {
		t.Fatalf("exit code = %d", code)
	}

	var out deployResultJSON
	dec := json.NewDecoder(strings.NewReader(stdout))
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}
	if dec.More() || out.Result != "build_failed" {
		t.Errorf("stdout:\n%s", stdout)
	}
}
