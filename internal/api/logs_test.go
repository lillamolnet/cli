package api

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestLogsFixtureFollowsSpec(t *testing.T) {
	assertFollowsSpecFor(t, http.MethodGet, "/api/v1/apps/{id}/logs", 200, readFixture(t, "logs.json"))
}

func TestLogs(t *testing.T) {
	client, req := server(t, 200, string(readFixture(t, "logs.json")))

	lines, next, err := client.Logs(context.Background(), "fdf37127-5906-4c4c-9005-4da8cf355adb",
		LogsQuery{Since: 2 * time.Hour, Limit: 50, Text: "GET", After: "123"})
	if err != nil {
		t.Fatal(err)
	}

	q := req.URL.Query()
	if req.URL.Path != "/api/v1/apps/fdf37127-5906-4c4c-9005-4da8cf355adb/logs" ||
		q.Get("since") != "7200" || q.Get("limit") != "50" || q.Get("text") != "GET" || q.Get("after") != "123" {
		t.Errorf("request = %s?%s", req.URL.Path, req.URL.RawQuery)
	}

	if len(lines) != 3 || lines[0].Line != "GET / 200" || lines[1].Line != "ERROR something broke" {
		t.Fatalf("lines = %+v", lines)
	}
	if lines[2].TS != "1791626225878119123" || next != lines[2].TS {
		t.Errorf("last ts = %q, next = %q", lines[2].TS, next)
	}
	if lines[0].Time.IsZero() || lines[0].Container == "" {
		t.Errorf("first = %+v", lines[0])
	}
}

func TestLogsDefaults(t *testing.T) {
	client, req := server(t, 200, `{"meta":{"lines":[],"next_after":null}}`)

	lines, next, err := client.Logs(context.Background(), "x", LogsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.RawQuery != "" || len(lines) != 0 || next != "" {
		t.Errorf("query = %q, lines = %v, next = %q", req.URL.RawQuery, lines, next)
	}
}
