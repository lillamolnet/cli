package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Execute(args, Input{Reader: strings.NewReader("")}, &out, &errOut, func(string) string { return "" })
	return code, out.String(), errOut.String()
}

func TestNoArgumentsPrintsHelp(t *testing.T) {
	code, stdout, _ := run(t)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("stdout has no usage:\n%s", stdout)
	}
}

func TestUsageErrorsExitWith2(t *testing.T) {
	cases := [][]string{
		{"nope"},
		{"--nope"},
		{"version", "extra"},
		{"version", "--nope"},
	}

	for _, args := range cases {
		code, stdout, stderr := run(t, args...)

		if code != exitUsage {
			t.Errorf("%v: exit code = %d, want %d", args, code, exitUsage)
		}
		if stdout != "" {
			t.Errorf("%v: stdout should be empty, got %q", args, stdout)
		}
		if !strings.HasPrefix(stderr, "Error: ") {
			t.Errorf("%v: stderr = %q", args, stderr)
		}
	}
}

func TestVersion(t *testing.T) {
	code, stdout, stderr := run(t, "version")

	if code != exitOK || stderr != "" {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.HasPrefix(stdout, "lillamolnet ") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestVersionJSON(t *testing.T) {
	version, commit = "1.2.3", "0123456789abcdef"
	t.Cleanup(func() { version, commit = "", "" })

	code, stdout, _ := run(t, "version", "--json")
	if code != exitOK {
		t.Fatalf("exit code = %d", code)
	}

	var info versionInfo
	if err := json.Unmarshal([]byte(stdout), &info); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}
	if info.Version != "1.2.3" || info.Commit != "0123456789abcdef" || info.OS == "" || info.Arch == "" {
		t.Errorf("info = %+v", info)
	}
}

// Every command that takes <app> says what it is, and shows an example with a
// subdomain, so that nobody has to guess.
func TestCommandsWithAnAppExplainIt(t *testing.T) {
	root := newRootCommand(&options{getenv: func(string) string { return "" }})

	var check func(cmd *cobra.Command)
	checked := 0
	check = func(cmd *cobra.Command) {
		if strings.Contains(cmd.Use, "<app>") {
			checked++
			if !strings.Contains(cmd.Long, appArgHelp) {
				t.Errorf("%s does not explain <app>", cmd.CommandPath())
			}
			if !strings.Contains(cmd.Example, "my-blog") {
				t.Errorf("%s has no example with a subdomain", cmd.CommandPath())
			}
		}
		for _, sub := range cmd.Commands() {
			check(sub)
		}
	}
	check(root)

	if checked < 9 {
		t.Errorf("only %d commands take <app>", checked)
	}
}

func TestVersionFlag(t *testing.T) {
	code, stdout, stderr := run(t, "--version")

	if code != exitOK || stderr != "" || !strings.HasPrefix(stdout, "lillamolnet ") {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

// The root help lists the exit codes, so that an agent need not find the
// README.
func TestRootHelpListsExitCodes(t *testing.T) {
	_, stdout, _ := run(t, "--help")

	for _, want := range []string{"Exit codes:", "  5   not found", "  10  deploy:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help has no %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "An app is named by its subdomain") {
		t.Errorf("help still explains <app>, which each command does:\n%s", stdout)
	}
}

// In a terminal, the banner comes first in the help for lillamolnet itself,
// and not in the help for a single command.
func TestBannerOnlyBeforeTheRootHelp(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	t.Cleanup(func() { isTerminal = defaultIsTerminal })

	for _, args := range [][]string{nil, {"--help"}} {
		_, stdout, _ := run(t, args...)
		banner := strings.Index(stdout, "~ lilla molnet ~")
		if banner < 0 || banner > strings.Index(stdout, "lillamolnet is the command line interface") {
			t.Errorf("%v: the banner is not first:\n%s", args, stdout)
		}
	}

	_, stdout, _ := run(t, "deploy", "--help")
	if strings.Contains(stdout, "~ lilla molnet ~") {
		t.Errorf("deploy --help has the banner:\n%s", stdout)
	}
}

// Outside a terminal, as when an agent reads the help, there is no banner.
func TestNoBannerOutsideATerminal(t *testing.T) {
	_, stdout, _ := run(t, "--help")
	if strings.Contains(stdout, "~ lilla molnet ~") {
		t.Errorf("the help has the banner outside a terminal:\n%s", stdout)
	}
}
