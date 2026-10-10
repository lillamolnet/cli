package cli

import (
	"errors"
	"net/http"
	"strings"

	"github.com/lillamolnet/cli/internal/api"
)

// The configuration is read from the environment, so that CI and agents can
// set it without files. A login command that stores the key comes later.
const (
	envToken  = "LILLAMOLNET_TOKEN"
	envAPIURL = "LILLAMOLNET_API_URL"
)

// authError means there is no usable API key.
type authError struct{ msg string }

func (e authError) Error() string { return e.msg }

var errNoToken = authError{
	"no API key. Create one at https://lillamolnet.se/users/settings/api_keys " +
		"and set it as " + envToken,
}

// client returns an API client from the environment.
func (o *options) client() (*api.Client, error) {
	token := strings.TrimSpace(o.getenv(envToken))
	if token == "" {
		return nil, errNoToken
	}

	baseURL := strings.TrimSpace(o.getenv(envAPIURL))
	if baseURL == "" {
		baseURL = api.DefaultBaseURL
	}

	return api.New(baseURL, token, currentVersion().Version)
}

// exitCode maps an error to the documented exit codes.
func exitCode(err error) int {
	var usage usageError
	var auth authError
	var notFound notFoundError
	var conn *api.ConnectionError
	var coded codeError
	var already *api.AlreadyBuildingError

	switch {
	case errors.As(err, &coded):
		return coded.code
	case errors.As(err, &already):
		return exitAlreadyBuilding
	case errors.As(err, &usage):
		return exitUsage
	case errors.As(err, &auth), api.IsStatus(err, http.StatusUnauthorized):
		return exitAuth
	case api.IsStatus(err, http.StatusForbidden):
		return exitForbidden
	case errors.As(err, &notFound), api.IsStatus(err, http.StatusNotFound):
		return exitNotFound
	case errors.As(err, &conn):
		return exitUnreachable
	default:
		return exitError
	}
}
