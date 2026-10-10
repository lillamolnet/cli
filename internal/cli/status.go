package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/lillamolnet/cli/internal/api"
	"github.com/spf13/cobra"
)

// statusJSON is the --json output of status. It is a stable contract.
type statusJSON struct {
	App          appJSON     `json:"app"`
	LatestBuild  *buildJSON  `json:"latest_build"`
	LatestDeploy *deployJSON `json:"latest_deploy"`
}

type buildJSON struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	Branch      *string    `json:"branch"`
	Commit      *string    `json:"commit"`
	Trigger     *string    `json:"trigger"`
	ErrorReason *string    `json:"error_reason"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}

type deployJSON struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	ErrorReason *string    `json:"error_reason"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
	Build       *buildJSON `json:"build"`
}

// notFoundError means that something the user named does not exist.
type notFoundError struct{ msg string }

func (e notFoundError) Error() string { return e.msg }

func newStatusCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:         "status <app>",
		Annotations: map[string]string{scopeAnnotation: "read"},
		Short:       "Show an app with its latest build and deploy",
		Long: "Show an app with its latest build and its latest deploy.\n\n" +
			"The latest deploy can belong to an older build than the latest, for example\n" +
			"when the latest build failed.",
		Example: "  lillamolnet status my-blog\n  lillamolnet status my-blog --json",
		Args:    exactlyOne("app"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := opts.client()
			if err != nil {
				return err
			}

			app, err := resolveApp(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}

			status, err := client.AppStatus(cmd.Context(), app.ID)
			if err != nil {
				return err
			}

			if opts.json {
				return json.NewEncoder(opts.stdout).Encode(statusJSON{
					App:          toAppJSON(status.App),
					LatestBuild:  toBuildJSON(status.LatestBuild),
					LatestDeploy: toDeployJSON(status.LatestDeploy),
				})
			}
			return printStatus(opts.stdout, status)
		},
	}
}

// resolveApp finds an app by id, subdomain or name, in that order.
func resolveApp(ctx context.Context, client *api.Client, ref string) (api.App, error) {
	apps, err := client.Apps(ctx)
	if err != nil {
		return api.App{}, err
	}

	for _, app := range apps {
		if app.ID == ref || app.Subdomain == ref {
			return app, nil
		}
	}

	var named []api.App
	for _, app := range apps {
		if strings.EqualFold(app.Name, ref) {
			named = append(named, app)
		}
	}

	switch len(named) {
	case 1:
		return named[0], nil
	case 0:
		return api.App{}, notFoundError{fmt.Sprintf("no app %q. Run 'lillamolnet apps' to list your apps", ref)}
	default:
		subdomains := make([]string, len(named))
		for i, app := range named {
			subdomains[i] = app.Subdomain
		}
		return api.App{}, usageError{fmt.Errorf("several apps are named %q. Use the subdomain: %s",
			ref, strings.Join(subdomains, ", "))}
	}
}

func printStatus(out io.Writer, status api.AppStatus) error {
	app := status.App
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "App:\t%s (%s)\n", app.Name, app.Subdomain)
	if app.URL != "" {
		fmt.Fprintf(w, "URL:\t%s\n", app.URL)
	}
	fmt.Fprintf(w, "State:\t%s\n", state(app))
	fmt.Fprintf(w, "Repo:\t%s\n", app.RepoURL)

	if b := status.LatestBuild; b != nil {
		fmt.Fprintf(w, "Latest build:\t%s\n", describeBuild(b))
		if b.ErrorReason != "" {
			fmt.Fprintf(w, "\t%s\n", b.ErrorReason)
		}
	} else {
		fmt.Fprintf(w, "Latest build:\tnone\n")
	}

	if d := status.LatestDeploy; d != nil {
		line := fmt.Sprintf("%s, %s", d.Status, formatTime(d.StartedAt))
		if d.Build != nil && d.Build.Commit != "" {
			line += ", commit " + shortCommit(d.Build.Commit)
		}
		fmt.Fprintf(w, "Latest deploy:\t%s\n", line)
		if d.ErrorReason != "" {
			fmt.Fprintf(w, "\t%s\n", d.ErrorReason)
		}
	} else {
		fmt.Fprintf(w, "Latest deploy:\tnone\n")
	}

	return w.Flush()
}

func describeBuild(b *api.Build) string {
	parts := []string{b.Status, formatTime(b.StartedAt)}
	if b.Commit != "" {
		parts = append(parts, "commit "+shortCommit(b.Commit))
	}
	if b.Branch != "" {
		parts = append(parts, "branch "+b.Branch)
	}
	return strings.Join(parts, ", ")
}

func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func toBuildJSON(b *api.Build) *buildJSON {
	if b == nil {
		return nil
	}
	return &buildJSON{
		ID:          b.ID,
		Status:      b.Status,
		Branch:      optional(b.Branch),
		Commit:      optional(b.Commit),
		Trigger:     optional(b.Trigger),
		ErrorReason: optional(b.ErrorReason),
		StartedAt:   b.StartedAt,
		FinishedAt:  b.FinishedAt,
	}
}

func toDeployJSON(d *api.Deploy) *deployJSON {
	if d == nil {
		return nil
	}
	return &deployJSON{
		ID:          d.ID,
		Status:      d.Status,
		ErrorReason: optional(d.ErrorReason),
		StartedAt:   d.StartedAt,
		FinishedAt:  d.FinishedAt,
		Build:       toBuildJSON(d.Build),
	}
}
