package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// loginServer answers the device flow with the token answers in order, then
// /me and the revocation of the current key.
type loginServer struct {
	url        string
	mu         sync.Mutex
	polls      int
	revoked    []string
	meKeys     []string
	clientName string
}

func newLoginServer(t *testing.T, tokenAnswers ...string) *loginServer {
	t.Helper()
	s := &loginServer{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		s.mu.Lock()
		defer s.mu.Unlock()

		switch r.URL.Path {
		case "/api/v1/device/codes":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.clientName = body["client_name"]
			_, _ = w.Write([]byte(fixture(t, "device_codes.json")))
		case "/api/v1/device/token":
			answer := tokenAnswers[min(s.polls, len(tokenAnswers)-1)]
			s.polls++
			if answer != "device_token.json" {
				w.WriteHeader(400)
			}
			_, _ = w.Write([]byte(fixture(t, answer)))
		case "/api/v1/me":
			s.meKeys = append(s.meKeys, r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(fixture(t, "me.json")))
		case "/api/v1/me/api_key":
			s.revoked = append(s.revoked, r.Header.Get("Authorization"))
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	s.url = srv.URL
	return s
}

// noWait makes polling instant, and records how long each wait would be.
func noWait(t *testing.T) *[]time.Duration {
	t.Helper()
	var waits []time.Duration
	sleep = func(_ context.Context, d time.Duration) { waits = append(waits, d) }
	t.Cleanup(func() {
		sleep = func(ctx context.Context, d time.Duration) {
			select {
			case <-ctx.Done():
			case <-time.After(d):
			}
		}
	})
	return &waits
}

// runLogin runs the CLI with a config directory of its own and no token.
func runLogin(t *testing.T, url, configDir string, extraEnv map[string]string, terminal bool, args ...string) (int, string, string) {
	t.Helper()
	values := map[string]string{envAPIURL: url, "XDG_CONFIG_HOME": configDir}
	for k, v := range extraEnv {
		values[k] = v
	}
	var out, errOut bytes.Buffer
	code := Execute(args, Input{Reader: strings.NewReader(""), Terminal: terminal}, &out, &errOut, env(values))
	return code, out.String(), errOut.String()
}

const exampleKey = "lillamolnet_EXAMPLEnotarealkey000000000000000000000000000000000000000000_abc123"

func TestLogin(t *testing.T) {
	waits := noWait(t)
	var opened []string
	openBrowser = func(url string) error { opened = append(opened, url); return nil }
	t.Cleanup(func() { openBrowser = defaultOpenBrowser })

	s := newLoginServer(t, "device_pending.json", "device_slow_down.json", "device_pending.json", "device_token.json")
	dir := t.TempDir()

	code, stdout, stderr := runLogin(t, s.url, dir, nil, true, "login")
	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}

	if !strings.Contains(stderr, "approve the code ZPQR-FDMQ") || len(opened) != 1 ||
		opened[0] != "https://lillamolnet.se/device?code=ZPQR-FDMQ" {
		t.Errorf("stderr = %q, opened = %v", stderr, opened)
	}
	if s.clientName == "" {
		t.Error("no client name was sent")
	}
	// 5 s, then 10 s after slow_down.
	if len(*waits) != 4 || (*waits)[0] != 5*time.Second || (*waits)[2] != 10*time.Second {
		t.Errorf("waits = %v", *waits)
	}
	if !strings.Contains(stdout, "Logged in as ada@example.com") || !strings.Contains(stdout, "read, deploy") ||
		strings.Contains(stdout, exampleKey) {
		t.Errorf("stdout = %q", stdout)
	}

	path := filepath.Join(dir, "lillamolnet", "credentials")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials: %v, mode %v", err, info.Mode())
	}
	saved, _ := os.ReadFile(path)
	if !strings.Contains(string(saved), exampleKey) || !strings.Contains(string(saved), s.url) {
		t.Errorf("saved = %s", saved)
	}

	// The saved key is used next time.
	code, stdout, _ = runLogin(t, s.url, dir, nil, false, "whoami")
	if code != exitOK || !strings.Contains(stdout, path) {
		t.Errorf("whoami: exit code = %d, stdout %q", code, stdout)
	}
	if last := s.meKeys[len(s.meKeys)-1]; last != "Bearer "+exampleKey {
		t.Errorf("whoami sent %q", last)
	}
}

func TestLoginDoesNotOpenABrowserWithoutATerminal(t *testing.T) {
	noWait(t)
	opened := false
	openBrowser = func(string) error { opened = true; return nil }
	t.Cleanup(func() { openBrowser = defaultOpenBrowser })

	s := newLoginServer(t, "device_token.json")
	if code, _, _ := runLogin(t, s.url, t.TempDir(), nil, false, "login"); code != exitOK || opened {
		t.Errorf("exit code = %d, opened = %v", code, opened)
	}
	if code, _, _ := runLogin(t, s.url, t.TempDir(), nil, true, "login", "--no-browser"); code != exitOK || opened {
		t.Errorf("--no-browser: exit code = %d, opened = %v", code, opened)
	}
}

func TestLoginDeniedOrExpired(t *testing.T) {
	noWait(t)
	for name, body := range map[string]string{
		"denied":  `{"errors":[{"status":"400","code":"access_denied","detail":"The code was denied"}]}`,
		"expired": fixture(t, "device_expired.json"),
	} {
		s := newLoginServerRaw(t, body)
		dir := t.TempDir()

		code, _, stderr := runLogin(t, s.url, dir, nil, false, "login")
		if code != exitAuth {
			t.Errorf("%s: exit code = %d, want %d (stderr %q)", name, code, exitAuth, stderr)
		}
		if _, err := os.Stat(filepath.Join(dir, "lillamolnet", "credentials")); err == nil {
			t.Errorf("%s: credentials were saved", name)
		}
	}
}

// newLoginServerRaw answers the token request with body.
func newLoginServerRaw(t *testing.T, body string) *loginServer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/device/codes" {
			_, _ = w.Write([]byte(fixture(t, "device_codes.json")))
			return
		}
		w.WriteHeader(400)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &loginServer{url: srv.URL}
}

func TestTokenEnvGoesBeforeSavedKey(t *testing.T) {
	noWait(t)
	s := newLoginServer(t, "device_token.json")
	dir := t.TempDir()
	runLogin(t, s.url, dir, nil, false, "login")

	code, stdout, _ := runLogin(t, s.url, dir, map[string]string{envToken: "lillamolnet_from_env"}, false, "whoami")
	if code != exitOK || !strings.Contains(stdout, envToken) {
		t.Errorf("exit code = %d, stdout %q", code, stdout)
	}
	if last := s.meKeys[len(s.meKeys)-1]; last != "Bearer lillamolnet_from_env" {
		t.Errorf("whoami sent %q", last)
	}
}

func TestSavedKeyIsOnlySentToItsAPI(t *testing.T) {
	noWait(t)
	s := newLoginServer(t, "device_token.json")
	dir := t.TempDir()
	runLogin(t, s.url, dir, nil, false, "login")

	other := newLoginServer(t, "device_token.json")
	code, _, stderr := runLogin(t, other.url, dir, nil, false, "whoami")

	if code != exitAuth || !strings.Contains(stderr, "the saved key is for") || len(other.meKeys) != 0 {
		t.Errorf("exit code = %d, stderr %q, other got %v", code, stderr, other.meKeys)
	}
}

func TestLogout(t *testing.T) {
	noWait(t)
	s := newLoginServer(t, "device_token.json")
	dir := t.TempDir()
	runLogin(t, s.url, dir, nil, false, "login")

	code, stdout, _ := runLogin(t, s.url, dir, nil, false, "logout")
	if code != exitOK || !strings.Contains(stdout, "The key is revoked") {
		t.Errorf("exit code = %d, stdout %q", code, stdout)
	}
	if len(s.revoked) != 1 || s.revoked[0] != "Bearer "+exampleKey {
		t.Errorf("revoked = %v", s.revoked)
	}
	if _, err := os.Stat(filepath.Join(dir, "lillamolnet", "credentials")); err == nil {
		t.Error("the credentials are still there")
	}

	code, stdout, _ = runLogin(t, s.url, dir, nil, false, "logout")
	if code != exitOK || !strings.Contains(stdout, "Not logged in") {
		t.Errorf("again: exit code = %d, stdout %q", code, stdout)
	}

	code, _, stderr := runLogin(t, s.url, dir, nil, false, "whoami")
	if code != exitAuth || !strings.Contains(stderr, "lillamolnet login") {
		t.Errorf("whoami: exit code = %d, stderr %q", code, stderr)
	}
}

func TestRefusedTokenEnvHintsAtSavedKey(t *testing.T) {
	noWait(t)
	s := newLoginServer(t, "device_token.json")
	dir := t.TempDir()
	runLogin(t, s.url, dir, nil, false, "login")

	refused := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"errors":[{"status":"401","code":"unauthorized","detail":"A valid API key is required"}]}`))
	}))
	t.Cleanup(refused.Close)

	// There is a saved key, and the key in LILLAMOLNET_TOKEN is refused.
	values := map[string]string{envAPIURL: refused.URL, "XDG_CONFIG_HOME": dir, envToken: "lillamolnet_old"}
	var out, errOut bytes.Buffer
	code := Execute([]string{"whoami"}, Input{Reader: strings.NewReader("")}, &out, &errOut, env(values))

	if code != exitAuth || !strings.Contains(errOut.String(), "unset LILLAMOLNET_TOKEN") {
		t.Errorf("exit code = %d, stderr %q", code, errOut.String())
	}
}
