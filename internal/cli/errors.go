package cli

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/lillamolnet/cli/internal/api"
)

// errorJSON is what --json prints on stdout when a command fails. It is a
// stable contract, like the rest of the JSON output. The same error is also
// printed as text on stderr.
type errorJSON struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	// Code names the exit code, such as not_found for 5.
	Code     string `json:"code"`
	Message  string `json:"message"`
	ExitCode int    `json:"exit_code"`
	// HTTPStatus is the API's status, when the API refused the call.
	HTTPStatus int `json:"http_status,omitempty"`
	// RequiredScopes are the scopes the command needs, when the key lacks one.
	RequiredScopes []string `json:"required_scopes,omitempty"`
}

// printedError is an error from a command that already printed its result,
// such as deploy --wait when the build failed. With --json, stdout then
// already holds the one JSON value, and no error object is added.
type printedError struct{ error }

func (e printedError) Unwrap() error { return e.error }

var exitCodeNames = map[int]string{
	exitError:           "error",
	exitUsage:           "usage",
	exitAuth:            "auth",
	exitForbidden:       "forbidden",
	exitNotFound:        "not_found",
	exitUnreachable:     "unreachable",
	exitBuildFailed:     "build_failed",
	exitDeployFailed:    "deploy_failed",
	exitTimeout:         "timeout",
	exitAlreadyBuilding: "already_building",
	exitCheckFailed:     "check_failed",
}

func writeErrorJSON(w io.Writer, err error, code int, scopes string) {
	body := errorBody{Code: exitCodeNames[code], Message: err.Error(), ExitCode: code}

	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		body.HTTPStatus = apiErr.Status
	}
	if code == exitForbidden && scopes != "" {
		body.RequiredScopes = strings.Split(scopes, ",")
	}

	_ = json.NewEncoder(w).Encode(errorJSON{Error: body})
}

// wantsJSON reports whether --json was given. A usage error can come before
// the flags are parsed, so the arguments are read too.
func wantsJSON(opts *options, args []string) bool {
	if opts.json {
		return true
	}
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--json" || arg == "--json=true" {
			return true
		}
	}
	return false
}
