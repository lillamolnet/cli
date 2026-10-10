package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"time"

	"github.com/lillamolnet/cli/internal/api"
	"github.com/spf13/cobra"
)

// logLineJSON is one line of the --json output of logs. It is a stable
// contract. The output has one such object per line, also with --follow.
type logLineJSON struct {
	TS        string    `json:"ts"`
	Time      time.Time `json:"time"`
	Line      string    `json:"line"`
	Container *string   `json:"container"`
}

// How often --follow asks for new lines. A variable, so that tests need not
// wait.
var followInterval = 2 * time.Second

var uuidPattern = regexp.MustCompile(`\A[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\z`)

func newLogsCommand(opts *options) *cobra.Command {
	var since time.Duration
	var limit int
	var grep string
	var follow bool

	cmd := &cobra.Command{
		Use:         "logs <app>",
		Annotations: map[string]string{scopeAnnotation: "read,logs:read"},
		Short:       "Print an app's logs",
		Long: "Print an app's logs, oldest first.\n\n" +
			"<app> is the app's subdomain, id or name. With the app's id, the API key only\n" +
			"needs the logs:read scope. A subdomain or name is looked up first, which also\n" +
			"needs read. Logs may contain personal data, which is why they have their own\n" +
			"scope.\n\n" +
			"With --follow, new lines are printed as they come, until Ctrl-C.",
		Args: exactlyOne("app"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if since <= 0 || since > api.MaxLogsSince {
				return usageError{fmt.Errorf("--since must be between 1s and %s", api.MaxLogsSince)}
			}
			if limit < 1 || limit > api.MaxLogsLimit {
				return usageError{fmt.Errorf("--limit must be between 1 and %d", api.MaxLogsLimit)}
			}

			client, err := opts.client()
			if err != nil {
				return err
			}

			appID := args[0]
			if !uuidPattern.MatchString(appID) {
				app, err := resolveApp(cmd.Context(), client, appID)
				if err != nil {
					return err
				}
				appID = app.ID
			}

			lines, next, err := client.Logs(cmd.Context(), appID, api.LogsQuery{Since: since, Limit: limit, Text: grep})
			if err != nil {
				return err
			}
			if err := printLogLines(opts, lines); err != nil {
				return err
			}

			if !follow {
				return nil
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			return followLogs(ctx, opts, client, appID, grep, next, since)
		},
	}

	cmd.Flags().DurationVar(&since, "since", time.Hour, "how far back, such as 15m or 24h, at most 168h")
	cmd.Flags().IntVar(&limit, "limit", 200, fmt.Sprintf("the most lines to print first, at most %d", api.MaxLogsLimit))
	cmd.Flags().StringVar(&grep, "grep", "", "only lines that contain the text")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "print new lines as they come, until Ctrl-C")
	return cmd
}

// followLogs asks for the lines after the last one until ctx is done. With no
// lines yet, it starts from now minus since, so that nothing is missed.
func followLogs(ctx context.Context, opts *options, client *api.Client, appID, grep, after string, since time.Duration) error {
	if after == "" {
		after = fmt.Sprint(time.Now().Add(-since).UnixNano())
	}
	failures := 0

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(followInterval):
		}

		lines, next, err := client.Logs(ctx, appID, api.LogsQuery{After: after, Limit: api.MaxLogsLimit, Text: grep})
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			failures++
			if !retryable(err) || failures >= maxWaitFailures {
				return err
			}
			fmt.Fprintf(opts.stderr, "Lost contact with the API (%s), trying again\n", err)
			continue
		}
		failures = 0

		if err := printLogLines(opts, lines); err != nil {
			return err
		}
		if next != "" {
			after = next
		}
	}
}

func printLogLines(opts *options, lines []api.LogLine) error {
	enc := json.NewEncoder(opts.stdout)
	for _, l := range lines {
		var err error
		if opts.json {
			err = enc.Encode(logLineJSON{TS: l.TS, Time: l.Time, Line: l.Line, Container: optional(l.Container)})
		} else {
			_, err = fmt.Fprintf(opts.stdout, "%s %s\n", l.Time.UTC().Format("2006-01-02T15:04:05.000Z"), l.Line)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
