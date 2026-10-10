package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lillamolnet/cli/internal/api"
)

const bakeryID = "fdf37127-5906-4c4c-9005-4da8cf355adb"

func TestLogs(t *testing.T) {
	url := apiServer(t, map[string]string{
		"/api/v1/apps":                       "apps_for_status.json",
		"/api/v1/apps/" + bakeryID + "/logs": "logs.json",
	}, 404, `{"errors":[{"status":"404","code":"not_found","detail":"not found"}]}`)

	code, stdout, stderr := runWithAPI(t, url, "logs", "bakery")
	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 || lines[0] != "2026-10-10T09:57:02.878Z GET / 200" || !strings.HasSuffix(lines[2], "GET /health 200") {
		t.Errorf("stdout:\n%s", stdout)
	}
}

func TestLogsJSONIsOneObjectPerLine(t *testing.T) {
	url := apiServer(t, map[string]string{"/api/v1/apps/" + bakeryID + "/logs": "logs.json"}, 404, "")

	code, stdout, _ := runWithAPI(t, url, "logs", bakeryID, "--json")
	if code != exitOK {
		t.Fatalf("exit code = %d", code)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 JSON lines:\n%s", stdout)
	}
	var first logLineJSON
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if first.Line != "GET / 200" || first.TS != "1791626222878119000" || first.Container == nil {
		t.Errorf("first = %+v", first)
	}
}

// With the app's id, the app list is not needed, so a key with only logs:read
// works. The server here answers 403 for the app list.
func TestLogsByIDNeedsNoAppList(t *testing.T) {
	url := apiServer(t, map[string]string{"/api/v1/apps/" + bakeryID + "/logs": "logs.json"}, 403,
		`{"errors":[{"status":"403","code":"forbidden","detail":"forbidden"}]}`)

	if code, _, stderr := runWithAPI(t, url, "logs", bakeryID); code != exitOK {
		t.Errorf("by id: exit code = %d, stderr %q", code, stderr)
	}
	if code, _, _ := runWithAPI(t, url, "logs", "bakery"); code != exitForbidden {
		t.Errorf("by subdomain: exit code = %d, want %d", code, exitForbidden)
	}
}

func TestLogsQueryFlags(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"meta":{"lines":[],"next_after":null}}`))
	}))
	t.Cleanup(srv.Close)

	code, _, _ := runWithAPI(t, srv.URL, "logs", bakeryID, "--since", "15m", "--limit", "50", "--grep", "ERROR")
	if code != exitOK || got != "limit=50&since=900&text=ERROR" {
		t.Errorf("exit code = %d, query = %q", code, got)
	}
}

func TestLogsBadFlags(t *testing.T) {
	url := apiServer(t, nil, 404, "")
	for _, args := range [][]string{
		{"--since", "0s"}, {"--since", "169h"}, {"--limit", "0"}, {"--limit", "1001"},
		{"--grep", "-i"}, {"--grep", "--ignore-case"},
	} {
		code, _, _ := runWithAPI(t, url, append([]string{"logs", bakeryID}, args...)...)
		if code != exitUsage {
			t.Errorf("%v: exit code = %d, want %d", args, code, exitUsage)
		}
	}
}

func TestLogsErrors(t *testing.T) {
	testAPIErrors(t, "logs", bakeryID)
}

func TestFollowLogs(t *testing.T) {
	followInterval = 10 * time.Millisecond
	t.Cleanup(func() { followInterval = 2 * time.Second })

	var mu sync.Mutex
	var afters []string
	answers := []string{
		`{"meta":{"lines":[{"ts":"101","time":"2026-10-10T10:00:01Z","line":"one"}],"next_after":"101"}}`,
		`{"meta":{"lines":[],"next_after":"101"}}`,
		`<html>Bad Gateway</html>`,
		`{"meta":{"lines":[{"ts":"102","time":"2026-10-10T10:00:02Z","line":"two"}],"next_after":"102"}}`,
	}

	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		afters = append(afters, r.URL.Query().Get("after"))
		answer := answers[min(len(afters)-1, len(answers)-1)]
		if strings.HasPrefix(answer, "<html>") {
			w.WriteHeader(502)
		}
		// The fifth request comes after "two" is printed.
		if len(afters) > len(answers) {
			cancel()
		}
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)

	client, err := api.New(srv.URL, "key", "dev")
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	opts := &options{stdout: &out, stderr: &errOut}

	if err := followLogs(ctx, opts, client, bakeryID, "", []api.LogLine{{TS: "100", Line: "zero"}}, time.Hour); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if strings.Join(afters, ",") != "99,100,100,100,101" {
		t.Errorf("after = %v", afters)
	}
	if !strings.Contains(out.String(), "one\n") || !strings.Contains(out.String(), "two\n") {
		t.Errorf("stdout = %q", out.String())
	}
	if !strings.Contains(errOut.String(), "trying again") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

// Lines with the very same timestamp are common in Loki. One that arrives
// after the others must still be printed, and none twice.
func TestLogCursorKeepsLinesWithTheSameTimestamp(t *testing.T) {
	line := func(ts, text string) api.LogLine { return api.LogLine{TS: ts, Line: text} }
	texts := func(lines []api.LogLine) string {
		var out []string
		for _, l := range lines {
			out = append(out, l.Line)
		}
		return strings.Join(out, ",")
	}

	c := newLogCursor(0)
	if got := texts(c.seen([]api.LogLine{line("4", "x"), line("5", "a")})); got != "x,a" {
		t.Fatalf("first = %q", got)
	}
	if c.after() != "4" {
		t.Errorf("after = %q, want just before the last timestamp", c.after())
	}

	// a again, and b with the same timestamp, which arrived later.
	if got := texts(c.seen([]api.LogLine{line("5", "a"), line("5", "b"), line("6", "c")})); got != "b,c" {
		t.Errorf("second = %q", got)
	}
	if got := texts(c.seen([]api.LogLine{line("6", "c"), line("6", "d")})); got != "d" {
		t.Errorf("third = %q", got)
	}
	// Two identical lines at one timestamp both count.
	if got := texts(c.seen([]api.LogLine{line("6", "c"), line("6", "d"), line("7", "e"), line("7", "e")})); got != "e,e" {
		t.Errorf("fourth = %q", got)
	}
	if got := texts(c.seen([]api.LogLine{line("7", "e"), line("7", "e")})); got != "" {
		t.Errorf("fifth = %q", got)
	}
}

// When the limit is reached, stderr says that older lines were left out.
func TestLogsSaysWhenTheLimitCutsLines(t *testing.T) {
	url := apiServer(t, map[string]string{"/api/v1/apps/" + bakeryID + "/logs": "logs.json"}, 404, "")

	code, stdout, stderr := runWithAPI(t, url, "logs", bakeryID, "--limit", "3")
	if code != exitOK || strings.Count(stdout, "\n") != 3 {
		t.Fatalf("exit code = %d, stdout:\n%s", code, stdout)
	}
	if !strings.Contains(stderr, "Only the newest 3 lines are shown") || !strings.Contains(stderr, "last 1h") {
		t.Errorf("stderr = %q", stderr)
	}

	_, _, stderr = runWithAPI(t, url, "logs", bakeryID)
	if stderr != "" {
		t.Errorf("under the limit, stderr = %q", stderr)
	}
}

// No lines is not an error, but stderr says that new lines can lag.
func TestLogsExplainsNoLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"meta":{"lines":[],"next_after":null}}`))
	}))
	t.Cleanup(srv.Close)

	code, stdout, stderr := runWithAPI(t, srv.URL, "logs", bakeryID, "--since", "10m", "--grep", "boom", "--json")
	if code != exitOK || stdout != "" {
		t.Fatalf("exit code = %d, stdout = %q", code, stdout)
	}
	if !strings.Contains(stderr, `No lines in the last 10m that contain "boom"`) || !strings.Contains(stderr, "up to a minute") {
		t.Errorf("stderr = %q", stderr)
	}
}
