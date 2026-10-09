// Package cli implements the lillamolnet commands.
package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Exit codes. They are part of the contract with scripts and agents, and are
// documented in the README.
const (
	exitOK          = 0
	exitError       = 1
	exitUsage       = 2
	exitAuth        = 3
	exitForbidden   = 4
	exitNotFound    = 5
	exitUnreachable = 6
)

// usageError is a mistake in how the command was called, such as an unknown
// command or flag.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

// scopeAnnotation names the API key scope a command needs, for the hint when
// the API answers 403.
const scopeAnnotation = "scope"

type options struct {
	json   bool
	stdout io.Writer
	stderr io.Writer
	getenv func(string) string
}

// Execute runs the command line in args and returns the exit code. Data goes
// to stdout and errors to stderr. getenv reads the configuration, as os.Getenv.
func Execute(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	opts := &options{stdout: stdout, stderr: stderr, getenv: getenv}
	root := newRootCommand(opts)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	cmd, err := root.ExecuteC()
	if err == nil {
		return exitOK
	}

	fmt.Fprintf(stderr, "Error: %s\n", err)

	code := exitCode(err)
	switch code {
	case exitUsage:
		fmt.Fprintf(stderr, "Run 'lillamolnet --help' for usage.\n")
	case exitForbidden:
		if scope := cmd.Annotations[scopeAnnotation]; scope != "" {
			fmt.Fprintf(stderr, "The API key needs the %s scope for this command. "+
				"Create a key with it at https://lillamolnet.se/users/settings/api_keys\n", scope)
		}
	}
	return code
}

func newRootCommand(opts *options) *cobra.Command {
	root := &cobra.Command{
		Use:   "lillamolnet",
		Short: "Deploy and inspect your apps on Lilla Molnet",
		Long: "lillamolnet is the command line interface for Lilla Molnet.\n\n" +
			"Every command takes --json for output that scripts and agents can parse.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError{fmt.Errorf("unknown command %q", args[0])}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.PersistentFlags().BoolVar(&opts.json, "json", false, "print output as JSON")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError{err}
	})
	root.CompletionOptions.HiddenDefaultCmd = true

	root.AddCommand(newVersionCommand(opts))
	root.AddCommand(newWhoamiCommand(opts))
	root.AddCommand(newAppsCommand(opts))
	root.AddCommand(newStatusCommand(opts))
	root.AddCommand(newBuildsCommand(opts))
	root.AddCommand(newBuildLogsCommand(opts))
	return root
}
