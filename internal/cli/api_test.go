package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtures is absolute, since some tests change the working directory.
var fixtures, _ = filepath.Abs("../api/testdata")

// apiServer serves fixture files from internal/api/testdata by path, and
// answers other paths with status.
func apiServer(t *testing.T, routes map[string]string, status int, body string) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		if fixture, ok := routes[r.URL.Path]; ok {
			data, err := os.ReadFile(filepath.Join(fixtures, fixture))
			if err != nil {
				t.Error(err)
			}
			_, _ = w.Write(data)
			return
		}

		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// loggedIn returns an environment for url with a saved key for it, in a
// config directory of the test's own.
func loggedIn(t *testing.T, url string) func(string) string {
	t.Helper()
	getenv := env(map[string]string{envAPIURL: url, "XDG_CONFIG_HOME": t.TempDir()})
	if _, err := saveCredentials(getenv, credentials{APIURL: url, APIKey: "lillamolnet_test_key", Name: "test"}); err != nil {
		t.Fatal(err)
	}
	return getenv
}

// runWithAPI runs the CLI against url, logged in.
func runWithAPI(t *testing.T, url string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	getenv := loggedIn(t, url)
	code = Execute(args, Input{Reader: strings.NewReader("")}, &out, &errOut, getenv)
	return code, out.String(), errOut.String()
}

// The same error cases for every command that calls the API.
func testAPIErrors(t *testing.T, args ...string) {
	t.Helper()

	cases := []struct {
		name   string
		status int
		body   string
		want   int
	}{
		{"invalid key", 401, `{"errors":[{"status":"401","code":"unauthorized","detail":"A valid API key is required."}]}`, exitAuth},
		{"missing scope", 403, `{"errors":[{"status":"403","code":"forbidden","detail":"forbidden"}]}`, exitForbidden},
		{"server error", 500, `{"errors":{"detail":"Internal Server Error"}}`, exitError},
	}

	for _, tc := range cases {
		url := apiServer(t, nil, tc.status, tc.body)
		code, stdout, stderr := runWithAPI(t, url, args...)

		if tc.want == exitForbidden && !strings.Contains(stderr, "The API key needs the ") {
			t.Errorf("%s: stderr has no hint about the scope: %q", tc.name, stderr)
		}

		if code != tc.want {
			t.Errorf("%s: exit code = %d, want %d (stderr %q)", tc.name, code, tc.want, stderr)
		}
		if stdout != "" {
			t.Errorf("%s: stdout should be empty, got %q", tc.name, stdout)
		}
	}

	t.Run("no key", func(t *testing.T) {
		var out, errOut bytes.Buffer
		code := Execute(args, Input{Reader: strings.NewReader("")}, &out, &errOut, env(nil))
		if code != exitAuth {
			t.Errorf("exit code = %d, want %d", code, exitAuth)
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()

		code, _, _ := runWithAPI(t, url, args...)
		if code != exitUnreachable {
			t.Errorf("exit code = %d, want %d", code, exitUnreachable)
		}
	})
}
