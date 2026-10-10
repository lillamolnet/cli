package cli

import (
	"errors"
	"fmt"
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
	"not logged in. Run lillamolnet login, or create a key at " +
		"https://lillamolnet.se/users/settings/api_keys and set it as " + envToken,
}

// client returns an API client from the environment.
// Where the key came from, for whoami.
const (
	keyFromEnv  = "env"
	keyFromFile = "credentials"
)

// client returns an API client with the key from LILLAMOLNET_TOKEN, or else
// the one lillamolnet login saved. A saved key is only sent to the API it was
// saved for, so that LILLAMOLNET_API_URL never leaks it to another server.
func (o *options) client() (*api.Client, error) {
	baseURL := o.baseURL()

	if token := strings.TrimSpace(o.getenv(envToken)); token != "" {
		o.keySource = keyFromEnv
		return api.New(baseURL, token, currentVersion().Version)
	}

	saved, err := loadCredentials(o.getenv)
	if err != nil {
		return nil, err
	}
	if saved == nil {
		return nil, errNoToken
	}
	if strings.TrimRight(saved.APIURL, "/") != baseURL {
		return nil, authError{fmt.Sprintf("the saved key is for %s, not %s. Run lillamolnet login, or set %s",
			saved.APIURL, baseURL, envToken)}
	}

	o.keySource = keyFromFile
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
