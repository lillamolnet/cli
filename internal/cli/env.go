package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/lillamolnet/cli/internal/api"
	"github.com/spf13/cobra"
)

// envVarJSON is a variable in the --json output of env. It is a stable
// contract, and never has the value.
type envVarJSON struct {
	Key           string     `json:"key"`
	Scope         string     `json:"scope"`
	Secret        bool       `json:"secret"`
	LastRotatedAt *time.Time `json:"last_rotated_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// envChangeJSON is the --json output of env set and env unset.
type envChangeJSON struct {
	App         string  `json:"app"`
	Key         string  `json:"key"`
	Result      string  `json:"result"`
	Scope       *string `json:"scope"`
	Secret      *bool   `json:"secret"`
	TakesEffect string  `json:"takes_effect"`
}

// The most a value read from stdin may be.
const maxEnvValue = 64 << 10

func newEnvCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "env <app>",
		Annotations: map[string]string{scopeAnnotation: "read"},
		Short:       "List, set and remove environment variables, never showing values",
		Long: "List an app's environment variables by key, with scope and whether they are\n" +
			"secret. The values are never shown, not even for variables that are not secret.",
		Example: "  lillamolnet env my-blog",
		Args:    exactlyOne("app"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := opts.client()
			if err != nil {
				return err
			}
			app, err := resolveApp(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}

			vars, err := client.EnvVars(cmd.Context(), app.ID)
			if err != nil {
				return err
			}

			if opts.json {
				out := make([]envVarJSON, 0, len(vars))
				for _, v := range vars {
					out = append(out, envVarJSON{
						Key: v.Key, Scope: v.Scope, Secret: v.Secret,
						LastRotatedAt: v.LastRotatedAt, CreatedAt: v.InsertedAt, UpdatedAt: v.UpdatedAt,
					})
				}
				return json.NewEncoder(opts.stdout).Encode(out)
			}

			if len(vars) == 0 {
				_, err := fmt.Fprintln(opts.stdout, "No environment variables.")
				return err
			}
			w := tabwriter.NewWriter(opts.stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "KEY\tSCOPE\tSECRET\tUPDATED")
			for _, v := range vars {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", v.Key, v.Scope, yesNo(v.Secret), formatTime(v.UpdatedAt))
			}
			return w.Flush()
		},
	}

	cmd.AddCommand(newEnvSetCommand(opts), newEnvUnsetCommand(opts))
	return cmd
}

func newEnvSetCommand(opts *options) *cobra.Command {
	var scope string
	var plain, yes bool

	cmd := &cobra.Command{
		Use: "set <app> KEY=VALUE",
		Example: "  lillamolnet env set my-blog PHX_HOST=my-blog.lillamolnet.se --plain --yes\n" +
			"  printf %s \"$DATABASE_URL\" | lillamolnet env set my-blog DATABASE_URL --yes",
		Annotations: map[string]string{scopeAnnotation: "read,env:write"},
		Short:       "Set an environment variable",
		Long: "Set an environment variable. A new key is created, a plain variable gets the new\n" +
			"value, and a secret one is rotated. It takes effect at the next deploy.\n\n" +
			"Give the value as KEY=VALUE, or only KEY and the value on stdin, so that it does\n" +
			"not end up in the shell's history:\n\n" +
			"  printf %s \"$DATABASE_URL\" | lillamolnet env set myapp DATABASE_URL --yes\n\n" +
			"New variables are secret unless --plain is given. A secret variable cannot\n" +
			"become plain. Asks before changing anything, unless --yes is given. Without a\n" +
			"terminal, --yes is required.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageError{fmt.Errorf("%s needs an app and KEY=VALUE, got %d arguments", cmd.CommandPath(), len(args))}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if scope != "" && scope != "build" && scope != "runtime" && scope != "both" {
				return usageError{fmt.Errorf("--scope must be build, runtime or both, got %q", scope)}
			}

			key, value, hasValue := strings.Cut(args[1], "=")
			if key == "" {
				return usageError{errors.New("the key is empty")}
			}
			if !hasValue {
				if opts.interactive {
					return usageError{fmt.Errorf("give the value as %s=VALUE, or pipe it on stdin", key)}
				}
				read, err := io.ReadAll(io.LimitReader(opts.stdin, maxEnvValue+1))
				if err != nil {
					return err
				}
				if len(read) > maxEnvValue {
					return usageError{fmt.Errorf("the value on stdin is larger than %d KB", maxEnvValue>>10)}
				}
				value = strings.TrimSuffix(string(read), "\n")
			}

			client, err := opts.client()
			if err != nil {
				return err
			}
			app, err := resolveApp(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}

			if err := confirm(opts, yes, fmt.Sprintf("Set %s on %s?", key, app.Subdomain)); err != nil {
				return err
			}

			set := api.SetEnvVar{Value: value, Scope: scope}
			if plain {
				secret := false
				set.Secret = &secret
			}

			v, result, err := client.SetEnvVar(cmd.Context(), app.ID, key, set)
			if api.IsStatus(err, http.StatusConflict) {
				return codeError{exitConflict, err.Error()}
			}
			if err != nil {
				return err
			}

			if opts.json {
				return json.NewEncoder(opts.stdout).Encode(envChangeJSON{
					App: app.Subdomain, Key: v.Key, Result: result,
					Scope: &v.Scope, Secret: &v.Secret, TakesEffect: "next_deploy",
				})
			}
			_, err = fmt.Fprintf(opts.stdout, "%s %s on %s. It takes effect at the next deploy: lillamolnet deploy %s\n",
				capitalize(result), v.Key, app.Subdomain, app.Subdomain)
			return err
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "build, runtime or both. A new variable gets both")
	cmd.Flags().BoolVar(&plain, "plain", false, "make a new variable plain rather than secret")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask before changing")
	return cmd
}

func newEnvUnsetCommand(opts *options) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:         "unset <app> KEY",
		Example:     "  lillamolnet env unset my-blog PHX_HOST --yes",
		Annotations: map[string]string{scopeAnnotation: "read,env:write"},
		Short:       "Remove an environment variable",
		Long: "Remove an environment variable. It takes effect at the next deploy.\n\n" +
			"Asks before removing, unless --yes is given. Without a terminal, --yes is\n" +
			"required.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageError{fmt.Errorf("%s needs an app and a KEY, got %d arguments", cmd.CommandPath(), len(args))}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[1]

			client, err := opts.client()
			if err != nil {
				return err
			}
			app, err := resolveApp(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}

			if err := confirm(opts, yes, fmt.Sprintf("Remove %s from %s?", key, app.Subdomain)); err != nil {
				return err
			}

			if err := client.UnsetEnvVar(cmd.Context(), app.ID, key); err != nil {
				return err
			}

			if opts.json {
				return json.NewEncoder(opts.stdout).Encode(envChangeJSON{
					App: app.Subdomain, Key: key, Result: "removed", TakesEffect: "next_deploy",
				})
			}
			_, err = fmt.Fprintf(opts.stdout, "Removed %s from %s. It takes effect at the next deploy: lillamolnet deploy %s\n",
				key, app.Subdomain, app.Subdomain)
			return err
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask before removing")
	return cmd
}

// errDeclined means the user answered no.
var errDeclined = codeError{exitError, "nothing was changed"}

// confirm asks the question on stderr and reads the answer, unless yes is
// given. Without a terminal it never asks, and yes is required.
func confirm(opts *options, yes bool, question string) error {
	if yes {
		return nil
	}
	if !opts.interactive {
		return usageError{errors.New("this changes the app, so it needs --yes when stdin is not a terminal")}
	}

	fmt.Fprintf(opts.stderr, "%s [y/N] ", question)
	answer, _ := bufio.NewReader(opts.stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	default:
		return errDeclined
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
