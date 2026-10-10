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

// Without arguments, lillamolnet says how to get started and where the rest
// is, not the full help.
func TestNoArgumentsPrintsTheIntro(t *testing.T) {
	code, stdout, stderr := run(t)

	if code != exitOK || stderr != "" {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "lillamolnet deploy my-blog --wait") || !strings.Contains(stdout, "lillamolnet --help") {
		t.Errorf("stdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "Usage:") || strings.Contains(stdout, "~ lilla molnet") {
		t.Errorf("stdout has the full help or the banner outside a terminal:\n%s", stdout)
	}
}

// In a terminal, the banner comes first without arguments, and never in
// --help.
func TestBannerOnlyWithoutArguments(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	t.Cleanup(func() { isTerminal = defaultIsTerminal })

	_, stdout, _ := run(t)
	banner := strings.Index(stdout, "~ lilla molnet")
	if banner < 0 || banner > strings.Index(stdout, "this is the command line interface for lilla molnet") {
		t.Errorf("the banner is not first:\n%s", stdout)
	}

	for _, args := range [][]string{{"--help"}, {"deploy", "--help"}} {
		if _, stdout, _ := run(t, args...); strings.Contains(stdout, "~ lilla molnet") {
			t.Errorf("%v has the banner:\n%s", args, stdout)
		}
	}
}

// The exit codes are a help topic, and --help says where it is.
func TestExitCodesHelpTopic(t *testing.T) {
	_, help, _ := run(t, "--help")
	if !strings.Contains(help, "lillamolnet help exit-codes") || strings.Contains(help, "  10  deploy:") {
		t.Errorf("--help:\n%s", help)
	}

	code, stdout, _ := run(t, "help", "exit-codes")
	for _, want := range []string{"Exit codes:", "  5   not found", "  10  deploy:"} {
		if code != exitOK || !strings.Contains(stdout, want) {
			t.Errorf("help exit-codes has no %q (exit code %d):\n%s", want, code, stdout)
		}
	}
}
