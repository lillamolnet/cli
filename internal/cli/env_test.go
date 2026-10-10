package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// envServer answers the app list and the env routes, and records changes.
type envServer struct {
	url      string
	mu       sync.Mutex
	requests []string
	bodies   []map[string]any
}

func newEnvServer(t *testing.T, putStatus int, putFixture string, deleteStatus int) *envServer {
	t.Helper()
	s := &envServer{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		base := "/api/v1/apps/" + bakeryID + "/env"

		switch {
		case r.URL.Path == "/api/v1/apps":
			_, _ = w.Write([]byte(fixture(t, "apps_for_status.json")))
		case r.Method == http.MethodGet && r.URL.Path == base:
			_, _ = w.Write([]byte(fixture(t, "env_list.json")))
		case strings.HasPrefix(r.URL.Path, base+"/"):
			s.mu.Lock()
			s.requests = append(s.requests, r.Method+" "+strings.TrimPrefix(r.URL.Path, base+"/"))
			body, _ := io.ReadAll(r.Body)
			var parsed map[string]any
			_ = json.Unmarshal(body, &parsed)
			s.bodies = append(s.bodies, parsed)
			s.mu.Unlock()

			if r.Method == http.MethodDelete {
				w.WriteHeader(deleteStatus)
				if deleteStatus == 404 {
					_, _ = w.Write([]byte(`{"errors":[{"status":"404","code":"not_found","detail":"No app or variable found"}]}`))
				}
				return
			}
			w.WriteHeader(putStatus)
			_, _ = w.Write([]byte(fixture(t, putFixture)))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	s.url = srv.URL
	return s
}

func (s *envServer) changes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

func (s *envServer) sentAttributes(i int) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bodies[i]["data"].(map[string]any)["attributes"].(map[string]any)
}

// runWithInput runs the CLI against url with stdin and whether it is a terminal.
func runWithInput(t *testing.T, url, stdin string, terminal bool, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	getenv := loggedIn(t, url)
	code = Execute(args, Input{Reader: strings.NewReader(stdin), Terminal: terminal}, &out, &errOut, getenv)
	return code, out.String(), errOut.String()
}

func TestEnvList(t *testing.T) {
	s := newEnvServer(t, 200, "env_rotated.json", 204)

	code, stdout, stderr := runWithAPI(t, s.url, "env", "bakery")
	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[1], "DATABASE_URL") || !strings.Contains(lines[1], "yes") ||
		!strings.HasPrefix(lines[2], "PHX_HOST") || !strings.Contains(lines[2], "no") {
		t.Errorf("stdout:\n%s", stdout)
	}

	code, stdout, _ = runWithAPI(t, s.url, "env", "bakery", "--json")
	var got []map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); code != exitOK || err != nil {
		t.Fatalf("exit code = %d, err = %v", code, err)
	}
	if len(got) != 2 || got[0]["key"] != "DATABASE_URL" || got[0]["secret"] != true {
		t.Errorf("got %v", got)
	}
	if _, ok := got[0]["value"]; ok {
		t.Error("the JSON has a value")
	}
}

func TestEnvSet(t *testing.T) {
	s := newEnvServer(t, 201, "env_created.json", 204)

	code, stdout, stderr := runWithAPI(t, s.url, "env", "set", "bakery", "SECRET_KEY_BASE=s3cret=with=equals", "--scope", "runtime", "--yes")
	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}

	if strings.Join(s.changes(), ",") != "PUT SECRET_KEY_BASE" {
		t.Errorf("requests = %v", s.changes())
	}
	if attrs := s.sentAttributes(0); attrs["value"] != "s3cret=with=equals" || attrs["scope"] != "runtime" || attrs["is_secret"] != nil {
		t.Errorf("attributes = %v", attrs)
	}
	if !strings.Contains(stdout, "Created SECRET_KEY_BASE on bakery") || !strings.Contains(stdout, "next deploy") {
		t.Errorf("stdout = %q", stdout)
	}
	if strings.Contains(stdout+stderr, "s3cret") {
		t.Error("the value was printed")
	}
}

func TestEnvSetFromStdin(t *testing.T) {
	s := newEnvServer(t, 200, "env_rotated.json", 204)

	code, stdout, stderr := runWithInput(t, s.url, "postgres://from-stdin\n", false, "env", "set", "bakery", "DATABASE_URL", "--yes", "--json")
	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	if attrs := s.sentAttributes(0); attrs["value"] != "postgres://from-stdin" {
		t.Errorf("value = %v", attrs["value"])
	}

	var got envChangeJSON
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || got.Result != "rotated" || got.TakesEffect != "next_deploy" {
		t.Errorf("got %+v, err %v", got, err)
	}
}

func TestEnvSetPlain(t *testing.T) {
	s := newEnvServer(t, 201, "env_created.json", 204)

	runWithAPI(t, s.url, "env", "set", "bakery", "PHX_HOST=example.com", "--plain", "--yes")

	if attrs := s.sentAttributes(0); attrs["is_secret"] != false {
		t.Errorf("is_secret = %v", attrs["is_secret"])
	}
}

func TestEnvSetAsks(t *testing.T) {
	s := newEnvServer(t, 201, "env_created.json", 204)

	code, _, stderr := runWithInput(t, s.url, "n\n", true, "env", "set", "bakery", "KEY=v")
	if code != exitError || !strings.Contains(stderr, "Set KEY on bakery? [y/N]") || len(s.changes()) != 0 {
		t.Errorf("no: exit code = %d, stderr %q, requests %v", code, stderr, s.changes())
	}

	code, _, _ = runWithInput(t, s.url, "y\n", true, "env", "set", "bakery", "KEY=v")
	if code != exitOK || len(s.changes()) != 1 {
		t.Errorf("yes: exit code = %d, requests %v", code, s.changes())
	}
}

func TestEnvSetNeedsYesWithoutATerminal(t *testing.T) {
	s := newEnvServer(t, 201, "env_created.json", 204)

	code, _, stderr := runWithInput(t, s.url, "", false, "env", "set", "bakery", "KEY=v")
	if code != exitUsage || !strings.Contains(stderr, "--yes") || len(s.changes()) != 0 {
		t.Errorf("exit code = %d, stderr %q, requests %v", code, stderr, s.changes())
	}
}

func TestEnvSetUsageErrors(t *testing.T) {
	s := newEnvServer(t, 201, "env_created.json", 204)

	for _, tc := range []struct {
		args     []string
		terminal bool
	}{
		{[]string{"env", "set", "bakery"}, false},
		{[]string{"env", "set", "bakery", "=v", "--yes"}, false},
		{[]string{"env", "set", "bakery", "KEY=v", "--scope", "everywhere", "--yes"}, false},
		{[]string{"env", "set", "bakery", "KEY"}, true},
	} {
		code, _, _ := runWithInput(t, s.url, "", tc.terminal, tc.args...)
		if code != exitUsage {
			t.Errorf("%v: exit code = %d, want %d", tc.args, code, exitUsage)
		}
	}
	if len(s.changes()) != 0 {
		t.Errorf("requests = %v", s.changes())
	}
}

func TestEnvSetRefused(t *testing.T) {
	for _, tc := range []struct {
		status  int
		fixture string
		stderr  string
	}{
		{409, "env_conflict.json", "A secret variable cannot become a plain one"},
		{422, "env_invalid.json", "key has invalid format"},
	} {
		s := newEnvServer(t, tc.status, tc.fixture, 204)

		code, _, stderr := runWithAPI(t, s.url, "env", "set", "bakery", "KEY=the-value", "--yes")

		if code != exitError || !strings.Contains(stderr, tc.stderr) || strings.Contains(stderr, "the-value") {
			t.Errorf("%d: exit code = %d, stderr %q", tc.status, code, stderr)
		}
	}
}

func TestEnvUnset(t *testing.T) {
	s := newEnvServer(t, 200, "env_rotated.json", 204)

	code, stdout, _ := runWithAPI(t, s.url, "env", "unset", "bakery", "PHX_HOST", "--yes")
	if code != exitOK || strings.Join(s.changes(), ",") != "DELETE PHX_HOST" || !strings.Contains(stdout, "Removed PHX_HOST from bakery") {
		t.Errorf("exit code = %d, requests %v, stdout %q", code, s.changes(), stdout)
	}

	s = newEnvServer(t, 200, "env_rotated.json", 404)
	if code, _, _ := runWithAPI(t, s.url, "env", "unset", "bakery", "NOPE", "--yes"); code != exitNotFound {
		t.Errorf("unknown key: exit code = %d, want %d", code, exitNotFound)
	}

	if code, _, _ := runWithInput(t, s.url, "", false, "env", "unset", "bakery", "PHX_HOST"); code != exitUsage {
		t.Errorf("without --yes: exit code = %d, want %d", code, exitUsage)
	}
}

func TestEnvErrors(t *testing.T) {
	testAPIErrors(t, "env", "bakery")
	testAPIErrors(t, "env", "set", "bakery", "KEY=v", "--yes")
	testAPIErrors(t, "env", "unset", "bakery", "KEY", "--yes")
}
