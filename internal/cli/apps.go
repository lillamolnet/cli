package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/lillamolnet/cli/internal/api"
	"github.com/spf13/cobra"
)

// appJSON is one app in the --json output of apps. It is a stable contract.
type appJSON struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Subdomain         string     `json:"subdomain"`
	Description       *string    `json:"description"`
	RepoURL           string     `json:"repo_url"`
	DefaultBranch     *string    `json:"default_branch"`
	AutoDeployEnabled bool       `json:"auto_deploy_enabled"`
	AutoDeployBranch  *string    `json:"auto_deploy_branch"`
	Stopped           bool       `json:"stopped"`
	StoppedAt         *time.Time `json:"stopped_at"`
	StoppedBy         *string    `json:"stopped_by"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func newAppsCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "apps",
		Short: "List your apps",
		Long: "List your apps, newest first.\n\n" +
			"STATE is the state you asked for: started, or stopped by you or by the platform.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := opts.client()
			if err != nil {
				return err
			}

			apps, err := client.Apps(cmd.Context())
			if err != nil {
				return err
			}

			if opts.json {
				out := make([]appJSON, 0, len(apps))
				for _, app := range apps {
					out = append(out, toAppJSON(app))
				}
				return json.NewEncoder(opts.stdout).Encode(out)
			}

			if len(apps) == 0 {
				_, err := fmt.Fprintln(opts.stdout, "No apps yet.")
				return err
			}

			w := tabwriter.NewWriter(opts.stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSUBDOMAIN\tSTATE\tAUTO DEPLOY\tREPO")
			for _, app := range apps {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					app.Name, app.Subdomain, state(app), autoDeploy(app), app.RepoURL)
			}
			return w.Flush()
		},
	}
}

func toAppJSON(app api.App) appJSON {
	return appJSON{
		ID:                app.ID,
		Name:              app.Name,
		Subdomain:         app.Subdomain,
		Description:       optional(app.Description),
		RepoURL:           app.RepoURL,
		DefaultBranch:     optional(app.DefaultBranch),
		AutoDeployEnabled: app.AutoDeployEnabled,
		AutoDeployBranch:  optional(app.AutoDeployBranch),
		Stopped:           app.StoppedAt != nil,
		StoppedAt:         app.StoppedAt,
		StoppedBy:         optional(app.StoppedBy),
		CreatedAt:         app.InsertedAt,
		UpdatedAt:         app.UpdatedAt,
	}
}

func state(app api.App) string {
	switch {
	case app.StoppedAt == nil:
		return "started"
	case app.StoppedBy == "platform":
		return "stopped by platform"
	default:
		return "stopped"
	}
}

func autoDeploy(app api.App) string {
	switch {
	case !app.AutoDeployEnabled:
		return "off"
	case app.AutoDeployBranch != "":
		return app.AutoDeployBranch
	default:
		return "on"
	}
}

// optional makes an empty string null in JSON.
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
