package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/lillamolnet/cli/internal/api"
	"github.com/spf13/cobra"
)

// deployResultJSON is the --json output of deploy. It is a stable contract.
type deployResultJSON struct {
	App    string      `json:"app"`
	URL    *string     `json:"url"`
	Build  *buildJSON  `json:"build"`
	Deploy *deployJSON `json:"deploy"`
	Done   bool        `json:"done"`
	Result string      `json:"result"`
}

// codeError is an error with its own exit code, such as a failed build.
type codeError struct {
	code int
	msg  string
}

func (e codeError) Error() string { return e.msg }

// How many failed wait requests in a row are tolerated, for example while
// the API itself is being deployed, and how long to pause between them. A
// variable, so that tests need not wait.
const maxWaitFailures = 5

var waitRetryPause = 3 * time.Second

func newDeployCommand(opts *options) *cobra.Command {
	var wait bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:         "deploy <app>",
		Annotations: map[string]string{scopeAnnotation: "read,deploy"},
		Short:       "Build and deploy an app",
		Long: "Build and deploy an app from its repository, like the Deploy button.\n\n" +
			"<app> is the app's subdomain, id or name. Without --wait, deploy returns as soon\n" +
			"as the build is queued. With --wait, it follows the build and the deploy to the\n" +
			"end, prints progress on stderr, and exits with a code that says how it went.\n" +
			"If a build of the app is already queued or running, no new one is started.",
		Args: exactlyOne("app"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if timeout <= 0 {
				return usageError{errors.New("--timeout must be positive")}
			}

			client, err := opts.client()
			if err != nil {
				return err
			}

			app, err := resolveApp(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}

			build, err := client.StartDeploy(cmd.Context(), app.ID)
			var already *api.AlreadyBuildingError
			if errors.As(err, &already) {
				return codeError{exitAlreadyBuilding, fmt.Sprintf(
					"a build of %s is already queued or running: %s. No new build was started",
					app.Subdomain, already.BuildID)}
			}
			if err != nil {
				return err
			}

			fmt.Fprintf(opts.stderr, "Queued build %s of %s\n", build.ID, app.Subdomain)

			result := api.WaitResult{Build: build, Result: api.ResultQueued}
			timedOut := false
			if wait {
				result, timedOut, err = waitForDeploy(cmd.Context(), client, opts.stderr, build.ID, timeout)
				if err != nil {
					return err
				}
			}

			if err := printDeploy(opts, app, result); err != nil {
				return err
			}
			return deployOutcome(result, timedOut, timeout)
		},
	}

	cmd.Flags().BoolVar(&wait, "wait", false, "follow the build and the deploy to the end")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "how long --wait waits, such as 10m")
	return cmd
}

// waitForDeploy asks the API to wait until the build and its deploy are done,
// or the timeout has passed.
func waitForDeploy(ctx context.Context, client *api.Client, progress io.Writer, buildID string, timeout time.Duration) (api.WaitResult, bool, error) {
	deadline := time.Now().Add(timeout)
	last := api.ResultQueued
	failures := 0

	for {
		remaining := time.Until(deadline).Truncate(time.Second)
		step := min(api.MaxWait, max(remaining, 0))

		result, err := client.Wait(ctx, buildID, step)
		if err != nil {
			failures++
			if !retryable(err) || failures >= maxWaitFailures {
				return api.WaitResult{}, false, err
			}
			fmt.Fprintf(progress, "Lost contact with the API (%s), trying again\n", err)
			time.Sleep(waitRetryPause)
			continue
		}
		failures = 0

		if result.Result != last {
			fmt.Fprintln(progress, progressLine(result))
			last = result.Result
		}
		if result.Done {
			return result, false, nil
		}
		if remaining <= 0 {
			return result, true, nil
		}
	}
}

// retryable reports whether a failed wait request may succeed if repeated.
func retryable(err error) bool {
	var conn *api.ConnectionError
	var apiErr *api.Error
	return errors.As(err, &conn) || (errors.As(err, &apiErr) && apiErr.Status >= 500)
}

func progressLine(r api.WaitResult) string {
	switch r.Result {
	case api.ResultBuilding:
		return "Building"
	case api.ResultDeploying:
		return "Deploying"
	case api.ResultDeployed:
		return "Deployed"
	case api.ResultBuildFailed:
		return "The build failed"
	case api.ResultDeployFailed:
		return "The deploy failed"
	default:
		return r.Result
	}
}

func printDeploy(opts *options, app api.App, r api.WaitResult) error {
	if opts.json {
		return json.NewEncoder(opts.stdout).Encode(deployResultJSON{
			App:    app.Subdomain,
			URL:    optional(app.URL),
			Build:  toBuildJSON(&r.Build),
			Deploy: toDeployJSON(r.Deploy),
			Done:   r.Done,
			Result: r.Result,
		})
	}

	switch r.Result {
	case api.ResultDeployed:
		line := "Deployed " + app.Subdomain
		if r.Build.Commit != "" {
			line += " at commit " + shortCommit(r.Build.Commit)
		}
		if app.URL != "" {
			line += ": " + app.URL
		}
		_, err := fmt.Fprintln(opts.stdout, line)
		return err
	case api.ResultBuildFailed:
		_, err := fmt.Fprintf(opts.stdout, "Build %s failed%s. See its log: lillamolnet build-logs %s\n",
			r.Build.ID, reason(r.Build.ErrorReason), r.Build.ID)
		return err
	case api.ResultDeployFailed:
		deployReason := ""
		if r.Deploy != nil {
			deployReason = reason(r.Deploy.ErrorReason)
		}
		_, err := fmt.Fprintf(opts.stdout, "The deploy of build %s failed%s. See its log: lillamolnet build-logs %s\n",
			r.Build.ID, deployReason, r.Build.ID)
		return err
	default:
		_, err := fmt.Fprintf(opts.stdout, "Build %s: %s\n", r.Build.ID, r.Result)
		return err
	}
}

func reason(s string) string {
	if s == "" {
		return ""
	}
	return ": " + s
}

// deployOutcome turns the result into the documented exit codes.
func deployOutcome(r api.WaitResult, timedOut bool, timeout time.Duration) error {
	switch {
	case r.Result == api.ResultBuildFailed:
		return codeError{exitBuildFailed, "the build failed"}
	case r.Result == api.ResultDeployFailed:
		return codeError{exitDeployFailed, "the deploy failed"}
	case timedOut:
		return codeError{exitTimeout, fmt.Sprintf("gave up waiting after %s. The build goes on: %s", timeout, r.Build.ID)}
	default:
		return nil
	}
}
