package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/lillamolnet/cli/internal/api"
	"github.com/spf13/cobra"
)

// buildLogJSON is the --json output of build-logs. It is a stable contract.
type buildLogJSON struct {
	ID  string `json:"id"`
	Log string `json:"log"`
}

func newBuildsCommand(opts *options) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:         "builds <app>",
		Annotations: map[string]string{scopeAnnotation: "read"},
		Short:       "List an app's builds, newest first",
		Long:        "List an app's builds, newest first. Use the build id with build-logs.",
		Example:     "  lillamolnet builds my-blog --limit 5",
		Args:        exactlyOne("app"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > api.MaxBuildsPerPage {
				return usageError{fmt.Errorf("--limit must be between 1 and %d", api.MaxBuildsPerPage)}
			}

			client, err := opts.client()
			if err != nil {
				return err
			}

			app, err := resolveApp(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}

			builds, err := client.Builds(cmd.Context(), app.ID, limit)
			if err != nil {
				return err
			}

			if opts.json {
				out := make([]*buildJSON, 0, len(builds))
				for i := range builds {
					out = append(out, toBuildJSON(&builds[i]))
				}
				return json.NewEncoder(opts.stdout).Encode(out)
			}
			return printBuilds(opts.stdout, builds)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, fmt.Sprintf("how many builds to show, at most %d", api.MaxBuildsPerPage))
	return cmd
}

func printBuilds(out io.Writer, builds []api.Build) error {
	if len(builds) == 0 {
		_, err := fmt.Fprintln(out, "No builds yet.")
		return err
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "STARTED\tSTATUS\tCOMMIT\tBRANCH\tTRIGGER\tID")
	for _, b := range builds {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			formatTime(b.StartedAt), b.Status, dash(shortCommit(b.Commit)), dash(b.Branch), dash(b.Trigger), b.ID)
	}
	return w.Flush()
}

func newBuildLogsCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use: "build-logs <build>",
		Example: "  lillamolnet build-logs 556cad24-fb5f-4ece-903c-3c534f7126d5\n" +
			"  lillamolnet build-logs $(lillamolnet builds my-blog --limit 1 --json | jq -r '.[0].id')",
		Annotations: map[string]string{scopeAnnotation: "read"},
		Short:       "Print a build's log",
		Long: "Print a build's log.\n\n" +
			"<build> is a build id, from builds or status. The log is printed as it is,\n" +
			"and is empty for a build without a log.",
		Args: exactlyOne("build"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !uuidPattern.MatchString(args[0]) {
				return usageError{fmt.Errorf("%q is not a build id. Build ids look like 556cad24-fb5f-4ece-903c-3c534f7126d5", args[0])}
			}

			client, err := opts.client()
			if err != nil {
				return err
			}

			log, err := client.BuildLog(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			if opts.json {
				return json.NewEncoder(opts.stdout).Encode(buildLogJSON{ID: args[0], Log: log})
			}

			if _, err := io.WriteString(opts.stdout, log); err != nil {
				return err
			}
			if log != "" && !strings.HasSuffix(log, "\n") {
				_, err = io.WriteString(opts.stdout, "\n")
			}
			return err
		},
	}
}

// exactlyOne requires one argument, named what in the error.
func exactlyOne(what string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return usageError{fmt.Errorf("%s needs exactly one %s, got %d", cmd.CommandPath(), what, len(args))}
		}
		return nil
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
