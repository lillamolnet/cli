// Package cli implements the lillamolnet commands.
package cli

import (
	"fmt"
	"io"
	"strings"

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
	// deploy --wait
	exitBuildFailed     = 7
	exitDeployFailed    = 8
	exitTimeout         = 9
	exitAlreadyBuilding = 10
)

// usageError is a mistake in how the command was called, such as an unknown
// command or flag.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

// scopeAnnotation names the API key scopes a command needs, separated by
// commas, for the hint when the API answers 403.
const scopeAnnotation = "scope"

// scopeList says "the read scope" or "the read and deploy scopes".
func scopeList(scopes string) string {
	parts := strings.Split(scopes, ",")
	if len(parts) == 1 {
		return "the " + parts[0] + " scope"
	}
	return "the " + strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1] + " scopes"
}

type options struct {
	json  bool
	stdin io.Reader
	// interactive is true when stdin is a terminal, so that a command may ask.
	interactive bool
	stdout      io.Writer
	stderr      io.Writer
	getenv      func(string) string
	// keySource is where the key came from, set by client.
	keySource string
}

// Input is where the CLI reads from.
type Input struct {
	Reader io.Reader
	// Terminal is true when Reader is a terminal.
	Terminal bool
}

// Execute runs the command line in args and returns the exit code. Data goes
// to stdout and errors to stderr. getenv reads the configuration, as os.Getenv.
func Execute(args []string, in Input, stdout, stderr io.Writer, getenv func(string) string) int {
	opts := &options{stdin: in.Reader, interactive: in.Terminal, stdout: stdout, stderr: stderr, getenv: getenv}
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
	case exitAuth:
		// An old LILLAMOLNET_TOKEN hides the key that login saved.
		if opts.keySource == keyFromEnv {
			if saved, _ := loadCredentials(getenv); saved != nil {
				fmt.Fprintf(stderr, "The key in %s was refused, and it is used before the key that "+
					"lillamolnet login saved. Unset it to use the saved key: unset %s\n", envToken, envToken)
			}
		}
	case exitForbidden:
		if scopes := cmd.Annotations[scopeAnnotation]; scopes != "" {
			fmt.Fprintf(stderr, "The API key needs %s for this command. "+
				"Create a key with it at https://lillamolnet.se/users/settings/api_keys\n", scopeList(scopes))
		}
	}
	return code
}

func newRootCommand(opts *options) *cobra.Command {
	// The commands in the order one uses them, not by name.
	cobra.EnableCommandSorting = false

	root := &cobra.Command{
		Use:   "lillamolnet",
		Short: "Deploy and inspect your apps on Lilla Molnet",
		Long: "lillamolnet is the command line interface for Lilla Molnet.\n\n" +
			"An app is named by its subdomain: my-blog for https://my-blog.lillamolnet.se.\n" +
			"lillamolnet apps lists them, with the subdomain first.\n\n" +
			"Get started:\n" +
			"  lillamolnet login\n" +
			"  lillamolnet apps\n" +
			"  lillamolnet status my-blog\n" +
			"  lillamolnet deploy my-blog --wait\n\n" +
			"Every command takes --json for output that scripts and agents can parse. In CI\n" +
			"and for agents, set LILLAMOLNET_TOKEN instead of logging in.",
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

	groups := []struct {
		id, title string
		commands  []*cobra.Command
	}{
		{"apps", "Apps:", []*cobra.Command{
			newAppsCommand(opts), newStatusCommand(opts), newDeployCommand(opts),
			newStopCommand(opts), newStartCommand(opts),
		}},
		{"builds", "Builds and logs:", []*cobra.Command{
			newBuildsCommand(opts), newBuildLogsCommand(opts), newLogsCommand(opts),
		}},
		{"config", "Configuration:", []*cobra.Command{newEnvCommand(opts)}},
		{"account", "Account:", []*cobra.Command{
			newLoginCommand(opts), newLogoutCommand(opts), newWhoamiCommand(opts), newVersionCommand(opts),
		}},
	}
	for _, g := range groups {
		root.AddGroup(&cobra.Group{ID: g.id, Title: g.title})
		for _, cmd := range g.commands {
			cmd.GroupID = g.id
			root.AddCommand(cmd)
		}
	}

	explainAppArg(root)
	return root
}

// appArgHelp says what <app> is. Every command that takes one gets it, so
// that the explanation is the same everywhere.
const appArgHelp = "<app> is the app's subdomain, as the SUBDOMAIN column of lillamolnet apps\n" +
	"shows it: my-blog for https://my-blog.lillamolnet.se. Its id or name works too."

func explainAppArg(cmd *cobra.Command) {
	if strings.Contains(cmd.Use, "<app>") {
		if cmd.Long == "" {
			cmd.Long = cmd.Short + "."
		}
		cmd.Long += "\n\n" + appArgHelp
	}
	for _, sub := range cmd.Commands() {
		explainAppArg(sub)
	}
}
