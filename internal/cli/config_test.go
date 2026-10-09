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

func TestClientNeedsAToken(t *testing.T) {
	for _, token := range []string{"", "   "} {
		opts := &options{getenv: env(map[string]string{envToken: token})}

		_, err := opts.client()
		if !errors.Is(err, errNoToken) {
			t.Errorf("token %q: err = %v", token, err)
		}
		if exitCode(err) != exitAuth {
			t.Errorf("token %q: exit code = %d", token, exitCode(err))
		}
	}
}

func TestClientRejectsABadAPIURL(t *testing.T) {
	opts := &options{getenv: env(map[string]string{envToken: "key", envAPIURL: "lillamolnet.se"})}

	if _, err := opts.client(); err == nil {
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
