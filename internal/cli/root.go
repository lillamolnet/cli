// Package cli implements the lillamolnet commands.
package cli

import (
	"errors"
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
	// check
	exitCheckFailed = 11
	// apps create, env set: the API refused because of a conflict, such as a
	// taken subdomain
	exitConflict = 12
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
	var printed printedError
	if wantsJSON(opts, args) && !errors.As(err, &printed) {
		writeErrorJSON(stdout, err, code, cmd.Annotations[scopeAnnotation])
	}
	switch code {
	case exitUsage:
		fmt.Fprintf(stderr, "Run 'lillamolnet --help' for usage.\n")
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
	var showVersion bool

	root := &cobra.Command{
		Use:   "lillamolnet",
		Short: "Deploy and inspect your apps on Lilla Molnet",
		Long: intro + "\n\n" +
			"Every command takes --json for output that scripts and agents can parse. When\n" +
			"a command fails, --json prints {\"error\": {\"code\", \"message\", \"exit_code\"}}.\n" +
			"The exit codes: lillamolnet help exit-codes\n" +
			"For coding agents: lillamolnet help agents\n" +
			"What a repository needs: lillamolnet help requirements",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError{fmt.Errorf("unknown command %q", args[0])}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				return printVersion(opts)
			}
			return printIntro(opts)
		},
	}

	root.PersistentFlags().BoolVar(&opts.json, "json", false, "print output as JSON")
	root.Flags().BoolVar(&showVersion, "version", false, "print the version")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError{err}
	})
	root.CompletionOptions.HiddenDefaultCmd = true

	groups := []struct {
		id, title string
		commands  []*cobra.Command
	}{
		{"apps", "Apps:", []*cobra.Command{
			newAppsCommand(opts), newStatusCommand(opts), newCheckCommand(opts), newDeployCommand(opts),
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

	root.AddCommand(&cobra.Command{
		Use:   "agents",
		Short: "How a coding agent deploys and debugs with lillamolnet",
		Long:  agentsHelp,
	})
	root.AddCommand(&cobra.Command{
		Use:   "requirements",
		Short: "What the platform expects of a repository",
		Long:  requirementsHelp,
	})
	root.AddCommand(&cobra.Command{
		Use:   "exit-codes",
		Short: "What the exit codes mean",
		Long:  exitCodeHelp,
	})

	explainAppArg(root)
	return root
}

// intro is what lillamolnet prints without arguments, and the start of its
// help.
const intro = "this is the command line interface for lilla molnet\n\n" +
	"Get started:\n" +
	"  lillamolnet login\n" +
	"  lillamolnet apps\n" +
	"  lillamolnet status my-blog\n" +
	"  lillamolnet deploy my-blog --wait"

// printIntro is lillamolnet without arguments: the banner in a terminal, how
// to get started, and where the rest is. The full help is in --help.
func printIntro(opts *options) error {
	if isTerminal(opts.stdout) {
		printBanner(opts.stdout)
	}
	_, err := fmt.Fprint(opts.stdout, intro+"\n\nRun 'lillamolnet --help' for all commands.\n")
	return err
}

// exitCodeHelp lists the exit codes, as the README does. It is the help
// topic lillamolnet help exit-codes.
const exitCodeHelp = "Exit codes:\n" +
	"  0   success\n" +
	"  1   unknown error\n" +
	"  2   wrong usage, such as an unknown command, flag or id\n" +
	"  3   not logged in, or the key is invalid, expired or revoked\n" +
	"  4   the key lacks a scope the command needs\n" +
	"  5   not found, such as an unknown app or build\n" +
	"  6   the API could not be reached\n" +
	"  7   deploy --wait: the build failed\n" +
	"  8   deploy --wait: the deploy failed\n" +
	"  9   deploy --wait: gave up after --timeout. The build goes on\n" +
	"  10  deploy: a build is already queued or running, so none was started\n" +
	"  11  check: a check failed\n" +
	"  12  apps create: the subdomain is taken. env set: a secret variable cannot\n" +
	"      become plain"

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
