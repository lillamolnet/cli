package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/lillamolnet/cli/internal/api"
	"github.com/spf13/cobra"
)

// The results of a check.
const (
	checkOK      = "ok"
	checkWarning = "warning"
	checkError   = "error"
)

// checkItem is one check in the --json output of check. It is a stable
// contract.
type checkItem struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// checkJSON is the --json output of check. It is a stable contract.
type checkJSON struct {
	// App is the app's subdomain, or null when no app deploys the repository.
	App *string `json:"app"`
	// Settings are what the repository was checked against: the app's, or
	// the platform's defaults.
	Settings settingsJSON `json:"settings"`
	// OK is false when a check has status error.
	OK     bool        `json:"ok"`
	Checks []checkItem `json:"checks"`
}

type settingsJSON struct {
	ContainerPort   int     `json:"container_port"`
	HealthCheckType string  `json:"health_check_type"`
	HealthCheckPath *string `json:"health_check_path"`
	HealthCheckPort *int    `json:"health_check_port"`
	MemoryLimitMB   int     `json:"memory_limit_mb"`
	CPULimit        float64 `json:"cpu_limit"`
	PidsLimit       int     `json:"pids_limit"`
}

// defaultSettings are what the platform uses for an app unless it has set
// others.
var defaultSettings = api.Settings{
	ContainerPort:   4000,
	HealthCheckType: "http",
	MemoryLimitMB:   512,
	CPULimit:        1,
	PidsLimit:       256,
}

func newCheckCommand(opts *options) *cobra.Command {
	var build bool
	var envFile string

	cmd := &cobra.Command{
		Use:         "check [<app>]",
		Annotations: map[string]string{scopeAnnotation: "read"},
		Short:       "Check that the repository here can be deployed, without deploying it",
		Long: "Check the repository in the current directory against what the platform\n" +
			"expects of it, without deploying it or changing anything on the platform.\n" +
			"lillamolnet help requirements lists what that is.\n\n" +
			"Without <app>, the app is the one whose repository is the git remote origin.\n" +
			"When no app has it, the repository is checked against the platform's\n" +
			"defaults.\n\n" +
			"Without --build, check reads the Dockerfile and git. With --build, it also\n" +
			"builds the image with Docker, as the platform does, and runs it with the\n" +
			"platform's PORT, capabilities and limits, followed by the same health\n" +
			"check. The app's env variables have no values here, so give the ones it\n" +
			"needs to start with --env-file.\n\n" +
			"Each check is ok, warning or error. check exits with 11 when a check is an\n" +
			"error.",
		Example: "  lillamolnet check\n" +
			"  lillamolnet check my-blog --json\n" +
			"  lillamolnet check --build --env-file .env.check",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usageError{fmt.Errorf("check takes at most one <app>, got %d arguments", len(args))}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if envFile != "" && !build {
				return usageError{errors.New("--env-file needs --build")}
			}

			client, err := opts.client()
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			repo := readRepo(ctx)

			var ref string
			if len(args) == 1 {
				ref = args[0]
			}
			app, appCheck, err := findApp(ctx, client, ref, repo)
			if err != nil {
				return err
			}

			settings := defaultSettings
			var envKeys []string
			if app != nil {
				settings = withDefaults(app.Settings)
				vars, err := client.EnvVars(ctx, app.ID)
				if err != nil {
					return err
				}
				for _, v := range vars {
					envKeys = append(envKeys, v.Key)
				}
			}

			checks := []checkItem{appCheck}
			checks = append(checks, repoChecks(repo, app)...)
			df, dockerfileCheck := readDockerfile(repo.root)
			checks = append(checks, dockerfileCheck)
			if df != nil {
				checks = append(checks, dockerfileChecks(*df, settings, envKeys)...)
				if build {
					final, _ := df.final()
					checks = append(checks, buildChecks(ctx, buildOptions{
						root:             repo.root,
						settings:         settings,
						envKeys:          envKeys,
						envFile:          envFile,
						dockerfileLabels: forbiddenLabels(labelKeys(final)),
						progress:         opts.stderr,
					})...)
					checks = connectPortAndHealth(checks, settings)
				}
			} else if build {
				checks = append(checks, checkItem{"build", checkWarning,
					"skipped, since there is no Dockerfile in the root to build. The run and the health check are skipped too"})
			}

			return printChecks(opts, app, settings, checks)
		},
	}

	cmd.Flags().BoolVar(&build, "build", false, "also build and run the image locally with Docker")
	cmd.Flags().StringVar(&envFile, "env-file", "", "env variables for the container, as docker run --env-file reads them")
	return cmd
}

func withDefaults(s api.Settings) api.Settings {
	// An API from before October 2026 has no settings.
	if s.ContainerPort == 0 {
		return defaultSettings
	}
	return s
}

func printChecks(opts *options, app *api.App, s api.Settings, checks []checkItem) error {
	var errorCount, warnings int
	for _, c := range checks {
		switch c.Status {
		case checkError:
			errorCount++
		case checkWarning:
			warnings++
		}
	}

	if opts.json {
		out := checkJSON{Settings: toSettingsJSON(s), OK: errorCount == 0, Checks: checks}
		if app != nil {
			out.App = &app.Subdomain
		}
		if err := json.NewEncoder(opts.stdout).Encode(out); err != nil {
			return err
		}
	} else if err := printCheckText(opts.stdout, checks, errorCount, warnings); err != nil {
		return err
	}

	if errorCount > 0 {
		return printedError{codeError{exitCheckFailed, plural(errorCount, "check", "checks") + " failed"}}
	}
	return nil
}

func printCheckText(out io.Writer, checks []checkItem, errorCount, warnings int) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, c := range checks {
		lines := strings.Split(c.Message, "\n")
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.Status, c.Name, lines[0])
		for _, line := range lines[1:] {
			fmt.Fprintf(w, "\t\t%s\n", line)
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}

	summary := "\nNo problems found."
	if errorCount+warnings > 0 {
		summary = fmt.Sprintf("\n%s, %s. lillamolnet help requirements lists what the platform expects.",
			plural(errorCount, "error", "errors"), plural(warnings, "warning", "warnings"))
	}
	_, err := fmt.Fprintln(out, summary)
	return err
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func toSettingsJSON(s api.Settings) settingsJSON {
	return settingsJSON{
		ContainerPort:   s.ContainerPort,
		HealthCheckType: s.HealthCheckType,
		HealthCheckPath: optional(s.HealthCheckPath),
		HealthCheckPort: nonZero(s.HealthCheckPort),
		MemoryLimitMB:   s.MemoryLimitMB,
		CPULimit:        s.CPULimit,
		PidsLimit:       s.PidsLimit,
	}
}

// repo is what git says about the repository in the current directory.
type repo struct {
	// root is the repository's top directory, or the current directory
	// outside git.
	root string
	git  bool
	// remote is the URL of origin, if there is one.
	remote string
	// branch is the checked out branch, or HEAD when none is.
	branch string
	// remoteDefault is the default branch of origin, as git knows it.
	remoteDefault string
	// dirty is true when there are uncommitted changes.
	dirty bool
	// unpushed is the number of commits not in the upstream branch, or -1
	// when the branch has no upstream.
	unpushed int
}

func readRepo(ctx context.Context) repo {
	git := func(args ...string) (string, bool) {
		out, err := exec.CommandContext(ctx, "git", args...).Output()
		return strings.TrimSpace(string(out)), err == nil
	}

	root, ok := git("rev-parse", "--show-toplevel")
	if !ok {
		wd, _ := os.Getwd()
		return repo{root: wd}
	}

	r := repo{root: root, git: true, unpushed: -1}
	r.remote, _ = git("remote", "get-url", "origin")
	r.branch, _ = git("rev-parse", "--abbrev-ref", "HEAD")
	if head, ok := git("symbolic-ref", "--short", "refs/remotes/origin/HEAD"); ok {
		r.remoteDefault = strings.TrimPrefix(head, "origin/")
	}
	if status, ok := git("status", "--porcelain"); ok {
		r.dirty = status != ""
	}
	if count, ok := git("rev-list", "--count", "@{upstream}..HEAD"); ok {
		r.unpushed, _ = strconv.Atoi(count)
	}
	return r
}

// findApp finds the app to check: the one named, or the one whose repository
// is the git remote. With neither, it returns no app and a warning.
func findApp(ctx context.Context, client *api.Client, ref string, r repo) (*api.App, checkItem, error) {
	if ref != "" {
		app, err := resolveApp(ctx, client, ref)
		if err != nil {
			return nil, checkItem{}, err
		}
		return &app, appFound(app, r), nil
	}

	noApp := func(msg string) (*api.App, checkItem, error) {
		return nil, checkItem{"app", checkWarning, msg + ". Checked against the platform's defaults"}, nil
	}
	if r.remote == "" {
		return noApp("no <app> given, and no git remote origin to find it from")
	}

	apps, err := client.Apps(ctx)
	if err != nil {
		return nil, checkItem{}, err
	}

	var matching []api.App
	for _, app := range apps {
		if sameRepo(app.RepoURL, r.remote) {
			matching = append(matching, app)
		}
	}

	switch len(matching) {
	case 0:
		return noApp("no app deploys " + r.remote)
	case 1:
		return &matching[0], appFound(matching[0], r), nil
	default:
		subdomains := make([]string, len(matching))
		for i, app := range matching {
			subdomains[i] = app.Subdomain
		}
		return nil, checkItem{}, usageError{fmt.Errorf("several apps deploy %s. Give one of them: %s",
			r.remote, strings.Join(subdomains, ", "))}
	}
}

func appFound(app api.App, r repo) checkItem {
	item := checkItem{"app", checkOK, fmt.Sprintf("%s deploys %s", app.Subdomain, app.RepoURL)}
	if r.remote != "" && !sameRepo(app.RepoURL, r.remote) {
		item.Status = checkWarning
		item.Message += fmt.Sprintf(", but the git remote origin here is %s", r.remote)
	}
	if app.Settings.ContainerPort == 0 {
		item.Message += ". The API does not give the app's settings, so the platform's defaults are used"
	}
	return item
}

var scpLikeURL = regexp.MustCompile(`^[\w.-]+@([\w.-]+):(.*)$`)

// sameRepo compares two repository URLs, so that
// git@github.com:a/b.git is the same as https://github.com/a/b.
func sameRepo(a, b string) bool {
	normalize := func(u string) string {
		u = strings.ToLower(strings.TrimSpace(u))
		if m := scpLikeURL.FindStringSubmatch(u); m != nil {
			u = m[1] + "/" + m[2]
		}
		if _, rest, ok := strings.Cut(u, "://"); ok {
			u = rest
		}
		// A user and token, as in https://user:token@github.com/a/b.
		if at := strings.Index(u, "@"); at >= 0 && !strings.Contains(u[:at], "/") {
			u = u[at+1:]
		}
		return strings.TrimSuffix(strings.TrimSuffix(u, "/"), ".git")
	}
	return normalize(a) != "" && normalize(a) == normalize(b)
}

// repoChecks are about git: that the branch here is the one the platform
// builds, and that it is pushed. The platform builds from the remote.
func repoChecks(r repo, app *api.App) []checkItem {
	if !r.git {
		return []checkItem{{"git", checkWarning,
			"not a git repository. The platform builds the app's repository on GitHub, not this directory"}}
	}

	var checks []checkItem
	if branches := deployedBranches(r, app); len(branches) > 0 {
		quoted := strings.Join(branches, " or ")
		if slices.Contains(branches, r.branch) {
			checks = append(checks, checkItem{"branch", checkOK, fmt.Sprintf("on %s, which the platform builds", r.branch)})
		} else {
			checks = append(checks, checkItem{"branch", checkWarning, fmt.Sprintf(
				"on %s, but the platform builds %s. What is checked here is not what a deploy builds", r.branch, quoted)})
		}
	}

	if r.remote == "" {
		return append(checks, checkItem{"pushed", checkWarning, "no git remote origin. The platform builds an " +
			"app's repository on GitHub, so a deploy needs this one pushed there"})
	}

	var notPushed []string
	if r.dirty {
		notPushed = append(notPushed, "uncommitted changes")
	}
	switch {
	case r.unpushed < 0:
		notPushed = append(notPushed, r.branch+" is not pushed")
	case r.unpushed > 0:
		notPushed = append(notPushed, plural(r.unpushed, "commit", "commits")+" not pushed")
	}
	if len(notPushed) == 0 {
		checks = append(checks, checkItem{"pushed", checkOK, "everything is committed and pushed"})
	} else {
		checks = append(checks, checkItem{"pushed", checkWarning, strings.Join(notPushed, ", ") +
			". The platform builds from the remote, so a deploy does not get what is only here"})
	}
	return checks
}

// deployedBranches are the branches the platform builds: the repository's
// default branch for the Deploy button and lillamolnet deploy, and the auto
// deploy branch for a push.
func deployedBranches(r repo, app *api.App) []string {
	var branches []string
	add := func(b string) {
		if b != "" && !slices.Contains(branches, b) {
			branches = append(branches, b)
		}
	}

	if app == nil {
		add(r.remoteDefault)
		return branches
	}

	if app.DefaultBranch != "" {
		add(app.DefaultBranch)
	} else {
		add(r.remoteDefault)
	}
	if app.AutoDeployEnabled {
		if app.AutoDeployBranch != "" {
			add(app.AutoDeployBranch)
		} else {
			add(app.DefaultBranch)
		}
	}
	return branches
}

// readDockerfile reads the Dockerfile in the root. The name must be exactly
// Dockerfile, also on a file system that ignores case, since the build server's
// does not.
func readDockerfile(root string) (*dockerfile, checkItem) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, checkItem{"dockerfile", checkError, err.Error()}
	}

	for _, e := range entries {
		if e.Name() == "Dockerfile" && !e.IsDir() {
			text, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
			if err != nil {
				return nil, checkItem{"dockerfile", checkError, err.Error()}
			}
			df := parseDockerfile(string(text))
			if len(df.stages) == 0 {
				return nil, checkItem{"dockerfile", checkError, "the Dockerfile has no FROM"}
			}
			return &df, checkItem{"dockerfile", checkOK, "Dockerfile in the repository root"}
		}
	}

	msg := "no Dockerfile in the repository root. The platform builds with docker build . in the root"
	if others := otherDockerfiles(root); len(others) > 0 {
		msg += ". Found " + strings.Join(others, ", ") + ", which the platform does not use"
	}
	return nil, checkItem{"dockerfile", checkError, msg}
}

// otherDockerfiles finds Dockerfiles that are not the one in the root, at most
// two directories down, for the error message.
func otherDockerfiles(root string) []string {
	var found []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel != "." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" ||
				strings.Count(rel, string(filepath.Separator)) >= 2) {
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		if (name == "dockerfile" || strings.HasPrefix(name, "dockerfile.") || strings.HasSuffix(name, ".dockerfile")) &&
			len(found) < 5 {
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	return found
}

// dockerfileChecks are the checks of the Dockerfile's contents.
func dockerfileChecks(df dockerfile, s api.Settings, envKeys []string) []checkItem {
	final, image := df.final()
	return []checkItem{
		portCheck(final, image, s),
		commandCheck(final),
		labelCheck(final),
		buildArgCheck(df, envKeys),
	}
}

// connectPortAndHealth joins what the Dockerfile suggests about the port
// with what --build showed. An answer on the app's port settles the port
// check. Without an answer, a problem the port check found is the likely
// cause.
func connectPortAndHealth(checks []checkItem, s api.Settings) []checkItem {
	port := slices.IndexFunc(checks, func(c checkItem) bool { return c.Name == "port" })
	health := slices.IndexFunc(checks, func(c checkItem) bool { return c.Name == "health_check" })
	if port < 0 || health < 0 || checks[port].Status == checkOK {
		return checks
	}

	switch {
	case checks[health].Status == checkOK && probePort(s) == s.ContainerPort:
		checks[port] = checkItem{"port", checkOK, fmt.Sprintf("the app answers on $PORT (%d), "+
			"as health_check shows, whatever the Dockerfile says", s.ContainerPort)}
	case checks[health].Status != checkOK:
		checks[health].Message += "\nThe likely cause is what port found: " + checks[port].Message
	}
	return checks
}

func portCheck(final []instruction, image string, s api.Settings) checkItem {
	port := s.ContainerPort
	usesPortVar := usesPort(final)
	exposed := exposedPorts(final)
	inCommand := commandPorts(command(final))
	item := func(status, format string, args ...any) checkItem {
		return checkItem{"port", status, fmt.Sprintf(format, args...)}
	}

	switch {
	case len(inCommand) > 0 && slices.Contains(inCommand, port):
		return item(checkOK, "the command listens on port %d, the app's port", port)
	case len(inCommand) > 0 && !usesPortVar:
		return item(checkError, "the command listens on port %d, but the platform sends traffic to port %d. "+
			"Listen on $PORT, which the platform sets to %d", inCommand[0], port, port)
	case slices.Contains(exposed, port):
		return item(checkOK, "EXPOSE %d is the app's port", port)
	case len(exposed) > 0 && !usesPortVar:
		return item(checkError, "EXPOSE %d, but the platform sends traffic to port %d, and the Dockerfile "+
			"never uses $PORT. Make the app listen on $PORT, which the platform sets to %d", exposed[0], port, port)
	case len(exposed) > 0:
		return item(checkWarning, "EXPOSE %d, but the platform sends traffic to port %d. The Dockerfile uses "+
			"PORT, and the platform sets PORT=%d when the container starts, replacing any ENV PORT in the "+
			"image. So it works if the app listens on $PORT. check --build tests it", exposed[0], port, port)
	case defaultPorts[imageName(image)] != 0 && defaultPorts[imageName(image)] != port && !usesPortVar:
		return item(checkError, "%s listens on port %d unless configured, but the platform sends traffic "+
			"to port %d. Make it listen on $PORT", imageName(image), defaultPorts[imageName(image)], port)
	case usesPortVar:
		return item(checkOK, "the Dockerfile uses $PORT, which the platform sets to %d", port)
	default:
		return item(checkWarning, "the Dockerfile does not say which port the app listens on. It must listen "+
			"on $PORT, which the platform sets to %d. check --build tests it", port)
	}
}

func commandCheck(final []instruction) checkItem {
	cmd := command(final)
	if cmd == "" {
		return checkItem{"command", checkWarning, "no CMD or ENTRYPOINT, so the base image's command starts the app"}
	}
	if m := loopback.FindString(cmd); m != "" {
		return checkItem{"command", checkWarning, fmt.Sprintf("the command mentions %s. The platform connects "+
			"from outside the container, so the app must listen on 0.0.0.0", m)}
	}
	return checkItem{"command", checkOK, "starts with " + cmd}
}

func labelCheck(final []instruction) checkItem {
	if forbidden := forbiddenLabels(labelKeys(final)); len(forbidden) > 0 {
		return checkItem{"labels", checkError, fmt.Sprintf("LABEL %s: the platform rejects an image with "+
			"labels that start with traefik. or spwnr. Routing and domains are set by the platform",
			strings.Join(forbidden, ", "))}
	}
	return checkItem{"labels", checkOK, "no labels that the platform controls"}
}

func buildArgCheck(df dockerfile, envKeys []string) checkItem {
	names := argsWithoutDefault(df)
	if len(names) == 0 {
		return checkItem{"build_args", checkOK, "no build args without a default"}
	}

	var envs []string
	for _, name := range names {
		if slices.Contains(envKeys, name) {
			envs = append(envs, name)
		}
	}
	msg := fmt.Sprintf("ARG %s without a default. The platform passes no build args, so they are empty in "+
		"the build", strings.Join(names, ", "))
	if len(envs) > 0 {
		msg += fmt.Sprintf(". %s is an env variable of the app, but env variables are only set when the "+
			"container starts", strings.Join(envs, ", "))
	}
	return checkItem{"build_args", checkWarning, msg}
}
