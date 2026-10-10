package cli

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/lillamolnet/cli/internal/api"
)

// envAPIURL points the CLI at another environment than the public API. The
// key always comes from lillamolnet login, never from the environment, so
// that it stays out of shells, logs and transcripts.
const envAPIURL = "LILLAMOLNET_API_URL"

// authError means there is no usable API key.
type authError struct{ msg string }

func (e authError) Error() string { return e.msg }

var errNoToken = authError{"not logged in. Run lillamolnet login"}

// client returns an API client with the key that lillamolnet login saved. It
// is only sent to the API it was saved for, so that LILLAMOLNET_API_URL never
// leaks it to another server.
func (o *options) client() (*api.Client, error) {
	baseURL := o.baseURL()

	saved, err := loadCredentials(o.getenv)
	if err != nil {
		return nil, err
	}
	if saved == nil {
		return nil, errNoToken
	}
	if strings.TrimRight(saved.APIURL, "/") != baseURL {
		return nil, authError{fmt.Sprintf("the saved key is for %s, not %s. Run lillamolnet login", saved.APIURL, baseURL)}
	}

	return api.New(baseURL, saved.APIKey, currentVersion().Version)
}

// baseURL is LILLAMOLNET_API_URL, or the public API.
func (o *options) baseURL() string {
	if u := strings.TrimSpace(o.getenv(envAPIURL)); u != "" {
		return strings.TrimRight(u, "/")
	}
	return api.DefaultBaseURL
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
