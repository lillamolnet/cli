package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// whoamiJSON is the --json output of whoami. It is a stable contract.
type whoamiJSON struct {
	ID     string     `json:"id"`
	Email  string     `json:"email"`
	APIKey apiKeyJSON `json:"api_key"`
	// KeySource is "env" for LILLAMOLNET_TOKEN or "credentials" for the key
	// that lillamolnet login saved.
	KeySource string `json:"key_source"`
}

type apiKeyJSON struct {
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt time.Time `json:"expires_at"`
}

func newWhoamiCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:         "whoami",
		Annotations: map[string]string{scopeAnnotation: "read"},
		Short:       "Show the user and the API key in use",
		Args:        noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := opts.client()
			if err != nil {
				return err
			}

			user, err := client.Me(cmd.Context())
			if err != nil {
				return err
			}

			if opts.json {
				return json.NewEncoder(opts.stdout).Encode(whoamiJSON{
					ID:    user.ID,
					Email: user.Email,
					APIKey: apiKeyJSON{
						Name:      user.APIKey.Name,
						Scopes:    nonNil(user.APIKey.Scopes),
						ExpiresAt: user.APIKey.ExpiresAt,
					},
					KeySource: opts.keySource,
				})
			}

			w := tabwriter.NewWriter(opts.stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "Email:\t%s\n", user.Email)
			fmt.Fprintf(w, "API key:\t%s\n", user.APIKey.Name)
			fmt.Fprintf(w, "Scopes:\t%s\n", strings.Join(user.APIKey.Scopes, ", "))
			fmt.Fprintf(w, "Expires:\t%s\n", user.APIKey.ExpiresAt.Format(time.DateOnly))
			fmt.Fprintf(w, "Key from:\t%s\n", keySourceText(opts))
			return w.Flush()
		},
	}
}

func keySourceText(opts *options) string {
	if opts.keySource == keyFromEnv {
		return envToken
	}
	if path, err := credentialsPath(opts.getenv); err == nil {
		return path
	}
	return "saved credentials"
}

// nonNil makes an empty list encode as [] rather than null.
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
