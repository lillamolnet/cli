package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/lillamolnet/cli/internal/api"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestClientNeedsALogin(t *testing.T) {
	opts := &options{getenv: env(map[string]string{"XDG_CONFIG_HOME": t.TempDir()})}

	_, err := opts.client()
	if !errors.Is(err, errNoToken) || exitCode(err) != exitAuth {
		t.Errorf("err = %v, exit code = %d", err, exitCode(err))
	}
}

// The key is never read from the environment, so that it stays out of
// shells, logs and transcripts.
func TestClientIgnoresATokenInTheEnvironment(t *testing.T) {
	opts := &options{getenv: env(map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "LILLAMOLNET_TOKEN": "lillamolnet_x"})}

	if _, err := opts.client(); !errors.Is(err, errNoToken) {
		t.Errorf("err = %v, want not logged in", err)
	}
}

func TestClientRejectsABadAPIURL(t *testing.T) {
	getenv := env(map[string]string{"XDG_CONFIG_HOME": t.TempDir(), envAPIURL: "lillamolnet.se"})
	if _, err := saveCredentials(getenv, credentials{APIURL: "lillamolnet.se", APIKey: "key"}); err != nil {
		t.Fatal(err)
	}

	if _, err := (&options{getenv: getenv}).client(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestExitCodes(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{usageError{errors.New("x")}, exitUsage},
		{errNoToken, exitAuth},
		{&api.Error{Status: 401}, exitAuth},
		{&api.Error{Status: 403}, exitForbidden},
		{&api.Error{Status: 404}, exitNotFound},
		{&api.Error{Status: 500}, exitError},
		{&api.ConnectionError{Err: errors.New("refused")}, exitUnreachable},
		{fmt.Errorf("wrapped: %w", &api.Error{Status: 403}), exitForbidden},
		{errors.New("anything else"), exitError},
	}

	for _, tc := range cases {
		if got := exitCode(tc.err); got != tc.want {
			t.Errorf("exitCode(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
}
