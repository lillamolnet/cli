package cli

import (
	"encoding/json"
	"fmt"

	"github.com/lillamolnet/cli/internal/api"
	"github.com/spf13/cobra"
)

// powerJSON is the --json output of stop and start. It is a stable contract.
type powerJSON struct {
	App    appJSON `json:"app"`
	Result string  `json:"result"`
}

func newStopCommand(opts *options) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:         "stop <app>",
		Example:     "  lillamolnet stop my-blog --yes",
		Annotations: map[string]string{scopeAnnotation: "read,deploy"},
		Short:       "Stop an app without removing it",
		Long: "Stop an app. Its containers stop, and it stays stopped until it is started. The\n" +
			"app, its subdomain and its domains are kept.\n\n" +
			"Asks before stopping, unless --yes is given. Without a terminal, --yes is\n" +
			"required.",
		Args: exactlyOne("app"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := opts.client()
			if err != nil {
				return err
			}
			app, err := resolveApp(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}

			if err := confirm(opts, yes, fmt.Sprintf("Stop %s? It goes offline until it is started.", app.Subdomain)); err != nil {
				return err
			}

			stopped, result, err := client.StopApp(cmd.Context(), app.ID)
			if err != nil {
				return err
			}
			return printPower(opts, stopped, result)
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask before stopping")
	return cmd
}

func newStartCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:         "start <app>",
		Example:     "  lillamolnet start my-blog",
		Annotations: map[string]string{scopeAnnotation: "read,deploy"},
		Short:       "Start a stopped app",
		Long: "Start a stopped app. The container that ran before starts again, or, if it is\n" +
			"gone, the same build is deployed again. An app that the platform stopped cannot\n" +
			"be started from here.",
		Args: exactlyOne("app"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := opts.client()
			if err != nil {
				return err
			}
			app, err := resolveApp(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}

			started, result, err := client.StartApp(cmd.Context(), app.ID)
			if err != nil {
				return err
			}
			return printPower(opts, started, result)
		},
	}
}

func printPower(opts *options, app api.App, result string) error {
	if opts.json {
		return json.NewEncoder(opts.stdout).Encode(powerJSON{App: toAppJSON(app), Result: result})
	}

	var line string
	switch result {
	case api.PowerStopped:
		line = fmt.Sprintf("Stopped %s. Start it again with: lillamolnet start %s", app.Subdomain, app.Subdomain)
	case api.PowerStarted:
		line = fmt.Sprintf("Started %s: %s", app.Subdomain, app.URL)
	case api.PowerRedeploying:
		line = fmt.Sprintf("Started %s. Its container was gone, so the same build is being deployed again. "+
			"Follow it with: lillamolnet status %s", app.Subdomain, app.Subdomain)
	case api.PowerNotDeployed:
		line = fmt.Sprintf("%s is no longer stopped, but it has never been deployed. Deploy it with: lillamolnet deploy %s",
			app.Subdomain, app.Subdomain)
	default:
		line = fmt.Sprintf("%s: %s", app.Subdomain, result)
	}
	_, err := fmt.Fprintln(opts.stdout, line)
	return err
}
