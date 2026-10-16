package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/lillamolnet/cli/internal/api"
	"github.com/spf13/cobra"
)

// autoDeployDefault is --auto-deploy without a branch: the repository's
// default branch. git does not allow a space in a branch, so it is never one.
const autoDeployDefault = "default branch"

func newAppsCreateCommand(opts *options) *cobra.Command {
	var repoFlag, name, subdomain, description, autoDeploy, integration string
	var yes bool

	cmd := &cobra.Command{
		Use: "create",
		Example: "  lillamolnet apps create --yes\n" +
			"  lillamolnet apps create --repo ada/blog --subdomain ada-blog --auto-deploy --yes",
		Annotations: map[string]string{scopeAnnotation: "read,apps:write"},
		Short:       "Create an app from a GitHub repository",
		Long: "Create an app from a GitHub repository. Without --repo, the repository is the\n" +
			"git remote origin of the one you are in. The platform chooses the servers, and\n" +
			"takes the repository's address and default branch from GitHub.\n\n" +
			"The name and the subdomain default to the repository's name. When the subdomain\n" +
			"is taken, the command fails with a free one to give with --subdomain, rather\n" +
			"than choosing one itself.\n\n" +
			"The Lilla Molnet GitHub app must be installed on the repository. That is done\n" +
			"in a browser, from https://lillamolnet.se/apps/new. When more than one of your\n" +
			"GitHub accounts reaches the repository, choose one with --integration.\n\n" +
			"The app is not deployed. Next: lillamolnet check --build, and\n" +
			"lillamolnet deploy <app> --wait.\n\n" +
			"Asks before creating, unless --yes is given. Without a terminal, --yes is\n" +
			"required.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fullName := repoFlag
			if fullName == "" {
				remote := readRepo(cmd.Context()).remote
				if remote == "" {
					return usageError{errors.New("no git remote origin here. Give the repository with --repo owner/repo")}
				}
				parsed, ok := githubFullName(remote)
				if !ok {
					return usageError{fmt.Errorf("the git remote origin %s is not on GitHub. "+
						"The platform builds from GitHub. Give the repository with --repo owner/repo", remote)}
				}
				fullName = parsed
			} else if !validFullName.MatchString(fullName) {
				return usageError{fmt.Errorf("--repo must be owner/repo, got %q", fullName)}
			}

			repoName := fullName[strings.Index(fullName, "/")+1:]
			if name == "" {
				name = repoName
			}
			if subdomain == "" {
				subdomain = subdomainFrom(repoName)
				if subdomain == "" {
					return usageError{fmt.Errorf("%q does not make a subdomain. Give one with --subdomain", repoName)}
				}
			}

			client, err := opts.client()
			if err != nil {
				return err
			}

			repo, err := findRepository(cmd, client, fullName, integration)
			if err != nil {
				return err
			}

			branch := autoDeploy
			if branch == autoDeployDefault {
				branch = repo.DefaultBranch
			}

			summary := fmt.Sprintf("Create %s at %s from %s", name, subdomain, repo.FullName)
			if branch != "" {
				summary += fmt.Sprintf(", deploying every push to %s", branch)
			}
			if err := confirm(opts, yes, summary+"?"); err != nil {
				return err
			}

			app, err := client.CreateApp(cmd.Context(), api.CreateApp{
				RepoID: repo.ID, IntegrationID: repo.IntegrationID,
				Name: name, Subdomain: subdomain, Description: description, AutoDeployBranch: branch,
			})
			if err != nil {
				return createError(err, subdomain)
			}

			if opts.json {
				return json.NewEncoder(opts.stdout).Encode(toAppJSON(app))
			}
			_, err = fmt.Fprintf(opts.stdout, "Created %s from %s: %s\n"+
				"It is not deployed yet. Next:\n"+
				"  lillamolnet check --build\n"+
				"  lillamolnet deploy %s --wait\n",
				app.Subdomain, repo.FullName, app.URL, app.Subdomain)
			return err
		},
	}

	cmd.Flags().StringVar(&repoFlag, "repo", "", "the GitHub repository, as owner/repo. Defaults to the git remote origin")
	cmd.Flags().StringVar(&name, "name", "", "the app's name. Defaults to the repository's name")
	cmd.Flags().StringVar(&subdomain, "subdomain", "", "the app's subdomain. Defaults to the repository's name")
	cmd.Flags().StringVar(&description, "description", "", "a description of the app")
	cmd.Flags().StringVar(&autoDeploy, "auto-deploy", "", "deploy every push to the branch. Without a branch, the default branch")
	cmd.Flags().Lookup("auto-deploy").NoOptDefVal = autoDeployDefault
	cmd.Flags().StringVar(&integration, "integration", "", "the GitHub integration's id, when more than one reaches the repository")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask before creating")
	return cmd
}

var validFullName = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)

// findRepository finds the repository among those the user's GitHub
// integrations reach.
func findRepository(cmd *cobra.Command, client *api.Client, fullName, integration string) (api.Repository, error) {
	repos, err := client.Repositories(cmd.Context())
	if err != nil {
		return api.Repository{}, err
	}

	var matching []api.Repository
	for _, r := range repos {
		if strings.EqualFold(r.FullName, fullName) && (integration == "" || r.IntegrationID == integration) {
			matching = append(matching, r)
		}
	}

	switch len(matching) {
	case 1:
		return matching[0], nil
	case 0:
		if integration != "" {
			return api.Repository{}, notFoundError{fmt.Sprintf("the GitHub integration %s does not reach %s", integration, fullName)}
		}
		return api.Repository{}, notFoundError{fmt.Sprintf("none of your GitHub accounts on Lilla Molnet reaches %s. "+
			"Install the Lilla Molnet GitHub app on it in a browser, from https://lillamolnet.se/apps/new, and try again", fullName)}
	default:
		ids := make([]string, len(matching))
		for i, r := range matching {
			ids[i] = r.IntegrationID
		}
		return api.Repository{}, usageError{fmt.Errorf("more than one of your GitHub accounts reaches %s. "+
			"Choose one with --integration: %s", fullName, strings.Join(ids, ", "))}
	}
}

// createError explains what the API answered when it did not create the app.
func createError(err error, subdomain string) error {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return err
	}

	switch {
	case apiErr.Status == http.StatusConflict && apiErr.Code == "subdomain_taken":
		msg := fmt.Sprintf("the subdomain %s is taken", subdomain)
		if free, ok := apiErr.Meta["suggested_subdomain"].(string); ok && free != "" {
			msg += fmt.Sprintf(". Try --subdomain %s", free)
		}
		return codeError{exitConflict, msg}
	case apiErr.Code == "repo_not_reachable":
		return notFoundError{"none of your GitHub accounts on Lilla Molnet reaches the repository. " +
			"Install the Lilla Molnet GitHub app on it in a browser, from https://lillamolnet.se/apps/new, and try again"}
	case apiErr.Code == "app_limit_reached", apiErr.Status == http.StatusServiceUnavailable:
		return codeError{exitError, apiErr.Detail}
	}
	return err
}

// githubFullName reads owner/repo from a GitHub remote, such as
// https://github.com/ada/blog.git or git@github.com:ada/blog.git.
func githubFullName(remote string) (string, bool) {
	u := strings.TrimSpace(remote)
	host, path := "", ""
	if m := scpLikeURL.FindStringSubmatch(u); m != nil && !strings.Contains(u, "://") {
		host, path = m[1], m[2]
	} else {
		_, rest, ok := strings.Cut(u, "://")
		if !ok {
			return "", false
		}
		// A user and token, as in https://user:token@github.com/a/b.
		if at := strings.Index(rest, "@"); at >= 0 && !strings.Contains(rest[:at], "/") {
			rest = rest[at+1:]
		}
		host, path, _ = strings.Cut(rest, "/")
	}

	if !strings.EqualFold(host, "github.com") {
		return "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if !validFullName.MatchString(path) {
		return "", false
	}
	return path, true
}

// subdomainFrom makes a subdomain of a repository's name: lowercase letters,
// digits and hyphens, 3 to 63 characters, or "" when that is not possible.
func subdomainFrom(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if s := b.String(); s != "" && !strings.HasSuffix(s, "-") {
				b.WriteRune('-')
			}
		}
	}

	s := b.String()
	if len(s) > 63 {
		s = s[:63]
	}
	s = strings.Trim(s, "-")
	if len(s) < 3 {
		return ""
	}
	return s
}
