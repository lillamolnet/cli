package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// powerServer answers the app list and stop and start, and records the calls.
func powerServer(t *testing.T, status int, answer string) (string, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var calls []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v1/apps":
			_, _ = w.Write([]byte(fixture(t, "apps_for_status.json")))
		case "/api/v1/apps/" + bakeryID + "/stop", "/api/v1/apps/" + bakeryID + "/start":
			mu.Lock()
			calls = append(calls, r.Method+" "+r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
			mu.Unlock()
			w.WriteHeader(status)
			_, _ = w.Write([]byte(fixture(t, answer)))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)

	return srv.URL, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), calls...)
	}
}

func TestStop(t *testing.T) {
	url, calls := powerServer(t, 200, "app_stopped.json")

	code, stdout, stderr := runWithAPI(t, url, "stop", "bakery", "--yes")
	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	if strings.Join(calls(), ",") != "POST stop" || !strings.Contains(stdout, "Stopped bakery") {
		t.Errorf("calls = %v, stdout = %q", calls(), stdout)
	}
}

func TestStopAsks(t *testing.T) {
	url, calls := powerServer(t, 200, "app_stopped.json")

	code, _, stderr := runWithInput(t, url, "n\n", true, "stop", "bakery")
	if code != exitError || !strings.Contains(stderr, "Stop bakery?") || len(calls()) != 0 {
		t.Errorf("no: exit code = %d, stderr %q, calls %v", code, stderr, calls())
	}

	if code, _, _ := runWithInput(t, url, "", false, "stop", "bakery"); code != exitUsage || len(calls()) != 0 {
		t.Errorf("without a terminal: exit code = %d, calls %v", code, calls())
	}

	if code, _, _ := runWithInput(t, url, "yes\n", true, "stop", "bakery"); code != exitOK || len(calls()) != 1 {
		t.Errorf("yes: exit code = %d, calls %v", code, calls())
	}
}

func TestStart(t *testing.T) {
	url, calls := powerServer(t, 200, "app_started.json")

	code, stdout, _ := runWithInput(t, url, "", false, "start", "bakery", "--json")
	if code != exitOK || strings.Join(calls(), ",") != "POST start" {
		t.Fatalf("exit code = %d, calls %v", code, calls())
	}

	var got powerJSON
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || got.Result != "started" || got.App.Stopped {
		t.Errorf("got %+v, err %v", got, err)
	}
}

func TestStartStoppedByPlatform(t *testing.T) {
	url, _ := powerServer(t, 409, "app_stopped_by_platform.json")

	code, _, stderr := runWithAPI(t, url, "start", "bakery")
	if code != exitError || !strings.Contains(stderr, "The platform stopped the app") {
		t.Errorf("exit code = %d, stderr %q", code, stderr)
	}
}

func TestPowerErrors(t *testing.T) {
	testAPIErrors(t, "stop", "bakery", "--yes")
	testAPIErrors(t, "start", "bakery")
}
