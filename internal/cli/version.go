package cli

import (
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Set at release time with -ldflags "-X github.com/lillamolnet/cli/internal/cli.version=...".
// Without them, the version comes from the module, as with go install.
var (
	version = ""
	commit  = ""
)

type versionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

func currentVersion() versionInfo {
	info := versionInfo{
		Version: version,
		Commit:  commit,
		Go:      runtime.Version(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}

	if build, ok := debug.ReadBuildInfo(); ok {
		if info.Version == "" {
			info.Version = build.Main.Version
		}
		for _, setting := range build.Settings {
			if setting.Key == "vcs.revision" && info.Commit == "" {
				info.Commit = setting.Value
			}
		}
	}

	if info.Version == "" || info.Version == "(devel)" {
		info.Version = "dev"
	}
	return info
}

func newVersionCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return printVersion(opts)
		},
	}
}

// printVersion prints the version, for version and --version.
func printVersion(opts *options) error {
	info := currentVersion()

	if opts.json {
		return json.NewEncoder(opts.stdout).Encode(info)
	}

	if info.Commit != "" {
		_, err := fmt.Fprintf(opts.stdout, "lillamolnet %s (%s, %s/%s)\n",
			info.Version, shortCommit(info.Commit), info.OS, info.Arch)
		return err
	}
	_, err := fmt.Fprintf(opts.stdout, "lillamolnet %s (%s/%s)\n", info.Version, info.OS, info.Arch)
	return err
}

func shortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// noArgs rejects arguments with a usage error.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageError{fmt.Errorf("%s takes no arguments, got %q", cmd.CommandPath(), args[0])}
	}
	return nil
}
