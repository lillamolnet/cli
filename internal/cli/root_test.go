package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Execute(args, &out, &errOut, func(string) string { return "" })
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
