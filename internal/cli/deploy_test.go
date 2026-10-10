package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const queuedBuildID = "560b8ba0-8027-449f-b695-d5a4c13077bc"

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("../api/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// waitAnswer is one answer to a wait request.
type waitAnswer struct {
	status int
	body   string
}

// withResult changes the result in a wait answer, and the build's status.
func withResult(t *testing.T, body, result, buildStatus string) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	doc["meta"] = map[string]any{"done": result == "deployed" || strings.HasSuffix(result, "_failed"), "result": result}
	doc["data"].(map[string]any)["attributes"].(map[string]any)["status"] = buildStatus
	if result == "build_failed" {
		doc["included"] = []any{}
		doc["data"].(map[string]any)["attributes"].(map[string]any)["error_reason"] = "docker_build_failed"
	}
	if result == "deploy_failed" {
		deploy := doc["included"].([]any)[0].(map[string]any)
		deploy["attributes"].(map[string]any)["status"] = "deploy_failed"
		deploy["attributes"].(map[string]any)["error_reason"] = "health_check_failed"
	}
	out, _ := json.Marshal(doc)
	return string(out)
}

// deployServer answers the app list, starts a deploy with startStatus, and
// gives the wait answers in order, repeating the last one.
func deployServer(t *testing.T, startStatus int, waits []waitAnswer) string {
	t.Helper()
	var mu sync.Mutex
	next := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch {
		case r.URL.Path == "/api/v1/apps":
			_, _ = w.Write([]byte(fixture(t, "apps_for_status.json")))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps/fdf37127-5906-4c4c-9005-4da8cf355adb/deploys":
			w.WriteHeader(startStatus)
			if startStatus == 409 {
				_, _ = w.Write([]byte(fixture(t, "deploy_conflict.json")))
			} else {
				_, _ = w.Write([]byte(fixture(t, "deploy_started.json")))
			}
		case r.URL.Path == "/api/v1/builds/"+queuedBuildID+"/wait":
			mu.Lock()
			answer := waits[min(next, len(waits)-1)]
			next++
			mu.Unlock()
			w.WriteHeader(answer.status)
			_, _ = w.Write([]byte(answer.body))
		default:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"errors":[{"status":"404","code":"not_found","detail":"not found"}]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestDeployWithoutWait(t *testing.T) {
	url := deployServer(t, 202, nil)

	code, stdout, stderr := runWithAPI(t, url, "deploy", "bakery")

	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stderr, "Queued build "+queuedBuildID) || !strings.Contains(stdout, queuedBuildID+": queued") {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
}

func TestDeployWait(t *testing.T) {
	queued := fixture(t, "wait_queued.json")
	deployed := fixture(t, "wait_deployed.json")
	url := deployServer(t, 202, []waitAnswer{
		{200, queued},
		{200, withResult(t, queued, "building", "running")},
		{200, deployed},
	})

	code, stdout, stderr := runWithAPI(t, url, "deploy", "bakery", "--wait")

	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	if strings.TrimSpace(stdout) != "Deployed bakery at commit def5678: https://bakery.lillamolnet.se" {
		t.Errorf("stdout = %q", stdout)
	}
	for _, want := range []string{"Queued build", "Building", "Deployed"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr has no %q: %q", want, stderr)
		}
	}
}

func TestDeployWaitJSON(t *testing.T) {
	url := deployServer(t, 202, []waitAnswer{{200, fixture(t, "wait_deployed.json")}})

	code, stdout, _ := runWithAPI(t, url, "deploy", "bakery", "--wait", "--json")
	if code != exitOK {
		t.Fatalf("exit code = %d", code)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}
	if got["result"] != "deployed" || got["done"] != true || got["app"] != "bakery" || got["url"] != "https://bakery.lillamolnet.se" {
		t.Errorf("got %v", got)
	}
	if build := got["build"].(map[string]any); build["commit"] != "def5678abc" || build["status"] != "success" {
		t.Errorf("build = %v", build)
	}
	if deploy := got["deploy"].(map[string]any); deploy["status"] != "deployed" {
		t.Errorf("deploy = %v", deploy)
	}
}

func TestDeployWaitFailures(t *testing.T) {
	deployed := fixture(t, "wait_deployed.json")

	cases := []struct {
		result, buildStatus string
		code                int
		stdout              string
	}{
		{"build_failed", "failed", exitBuildFailed, "failed: docker_build_failed. See its log: lillamolnet build-logs " + queuedBuildID},
		{"deploy_failed", "success", exitDeployFailed, "failed: health_check_failed. See its log"},
	}

	for _, tc := range cases {
		url := deployServer(t, 202, []waitAnswer{{200, withResult(t, deployed, tc.result, tc.buildStatus)}})

		code, stdout, stderr := runWithAPI(t, url, "deploy", "bakery", "--wait")

		if code != tc.code {
			t.Errorf("%s: exit code = %d, want %d (stderr %q)", tc.result, code, tc.code, stderr)
		}
		if !strings.Contains(stdout, tc.stdout) {
			t.Errorf("%s: stdout = %q", tc.result, stdout)
		}
	}
}

func TestDeployWaitTimeout(t *testing.T) {
	url := deployServer(t, 202, []waitAnswer{{200, fixture(t, "wait_queued.json")}})

	code, stdout, stderr := runWithAPI(t, url, "deploy", "bakery", "--wait", "--timeout", "1s")

	if code != exitTimeout {
		t.Errorf("exit code = %d, want %d", code, exitTimeout)
	}
	if !strings.Contains(stdout, "queued") || !strings.Contains(stderr, "gave up waiting after 1s") {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
}

func TestDeployWaitSurvivesAShortOutage(t *testing.T) {
	waitRetryPause = time.Millisecond
	t.Cleanup(func() { waitRetryPause = 3 * time.Second })

	url := deployServer(t, 202, []waitAnswer{
		{502, `<html>Bad Gateway</html>`},
		{502, `<html>Bad Gateway</html>`},
		{200, fixture(t, "wait_deployed.json")},
	})

	code, _, stderr := runWithAPI(t, url, "deploy", "bakery", "--wait")

	if code != exitOK {
		t.Errorf("exit code = %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stderr, "trying again") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestDeployWaitGivesUpAfterALongOutage(t *testing.T) {
	waitRetryPause = time.Millisecond
	t.Cleanup(func() { waitRetryPause = 3 * time.Second })

	url := deployServer(t, 202, []waitAnswer{{502, `<html>Bad Gateway</html>`}})

	code, _, _ := runWithAPI(t, url, "deploy", "bakery", "--wait")
	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
}

func TestDeployAlreadyBuilding(t *testing.T) {
	url := deployServer(t, 409, nil)

	code, stdout, stderr := runWithAPI(t, url, "deploy", "bakery", "--wait")

	if code != exitAlreadyBuilding {
		t.Errorf("exit code = %d, want %d", code, exitAlreadyBuilding)
	}
	if stdout != "" || !strings.Contains(stderr, queuedBuildID) {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
}

func TestDeployScopeHint(t *testing.T) {
	url := apiServer(t, map[string]string{"/api/v1/apps": "apps_for_status.json"}, 403,
		`{"errors":[{"status":"403","code":"forbidden","detail":"forbidden"}]}`)

	code, _, stderr := runWithAPI(t, url, "deploy", "bakery")

	if code != exitForbidden || !strings.Contains(stderr, "needs the read and deploy scopes") {
		t.Errorf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestDeployBadTimeout(t *testing.T) {
	for _, timeout := range []string{"0s", "-1m", "soon"} {
		code, _, _ := runWithAPI(t, deployServer(t, 202, nil), "deploy", "bakery", "--wait", "--timeout", timeout)
		if code != exitUsage {
			t.Errorf("--timeout %s: exit code = %d, want %d", timeout, code, exitUsage)
		}
	}
}

func TestDeployErrors(t *testing.T) {
	testAPIErrors(t, "deploy", "bakery")
}
