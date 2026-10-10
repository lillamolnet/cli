package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"github.com/lillamolnet/cli/internal/api"
	"github.com/spf13/cobra"
)

// loginJSON is the --json output of login. It is a stable contract, and never
// has the key itself.
type loginJSON struct {
	Email       string     `json:"email"`
	APIKey      apiKeyJSON `json:"api_key"`
	Credentials string     `json:"credentials"`
}

// Replaceable in tests: how the browser is opened and how long a poll waits.
var (
	openBrowser = defaultOpenBrowser
	sleep       = func(ctx context.Context, d time.Duration) {
		select {
		case <-ctx.Done():
		case <-time.After(d):
		}
	}
)

func newLoginCommand(opts *options) *cobra.Command {
	var noBrowser bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in through the browser and save an API key",
		Long: "Log in through the browser. lillamolnet shows a code, you approve it on\n" +
			"lillamolnet.se and choose what the key may do, and the key is saved in\n" +
			"~/.config/lillamolnet/credentials, readable only by you.\n\n" +
			"In CI and for agents, set LILLAMOLNET_TOKEN instead. It is used before a saved\n" +
			"key.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL := opts.baseURL()
			client, err := api.New(baseURL, "", currentVersion().Version)
			if err != nil {
				return err
			}

			host, _ := os.Hostname()
			code, err := client.StartDeviceLogin(cmd.Context(), host)
			if err != nil {
				return err
			}

			fmt.Fprintf(opts.stderr, "Open %s and approve the code %s\n", code.VerificationURIComplete, code.UserCode)
			if opts.interactive && !noBrowser {
				if err := openBrowser(code.VerificationURIComplete); err == nil {
					fmt.Fprintln(opts.stderr, "The browser should open by itself.")
				}
			}
			fmt.Fprintln(opts.stderr, "Waiting for the approval...")

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()

			token, err := pollForKey(ctx, client, code)
			if err != nil {
				return err
			}

			path, err := saveCredentials(opts.getenv, credentials{
				APIURL: baseURL, APIKey: token.APIKey, Name: token.Name, ExpiresAt: token.ExpiresAt,
			})
			if err != nil {
				return fmt.Errorf("the key could not be saved: %w", err)
			}

			email := ""
			if keyed, err := api.New(baseURL, token.APIKey, currentVersion().Version); err == nil {
				if user, err := keyed.Me(cmd.Context()); err == nil {
					email = user.Email
				}
			}

			if strings.TrimSpace(opts.getenv(envToken)) != "" {
				fmt.Fprintf(opts.stderr, "Note: %s is set, and is used before the saved key.\n", envToken)
			}

			if opts.json {
				return json.NewEncoder(opts.stdout).Encode(loginJSON{
					Email:       email,
					APIKey:      apiKeyJSON{Name: token.Name, Scopes: nonNil(token.Scopes), ExpiresAt: token.ExpiresAt},
					Credentials: path,
				})
			}

			who := "Logged in"
			if email != "" {
				who += " as " + email
			}
			_, err = fmt.Fprintf(opts.stdout, "%s with the key %q (%s), valid until %s. Saved in %s\n",
				who, token.Name, strings.Join(token.Scopes, ", "), token.ExpiresAt.Format(time.DateOnly), path)
			return err
		},
	}

	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "only print the link, do not open the browser")
	return cmd
}

// pollForKey asks for the key every interval until the code is approved,
// denied or expired. slow_down adds five seconds, as RFC 8628 says.
func pollForKey(ctx context.Context, client *api.Client, code api.DeviceCode) (api.DeviceToken, error) {
	interval := max(code.Interval, time.Second)
	deadline := time.Now().Add(code.ExpiresIn)
	failures := 0

	for time.Now().Before(deadline) {
		sleep(ctx, interval)
		if ctx.Err() != nil {
			return api.DeviceToken{}, codeError{exitError, "the login was interrupted"}
		}

		token, err := client.PollDeviceToken(ctx, code.DeviceCode)
		switch api.DeviceErrorCode(err) {
		case "":
			if err == nil {
				return token, nil
			}
			failures++
			if !retryable(err) || failures >= maxWaitFailures {
				return api.DeviceToken{}, err
			}
			continue
		case api.DeviceAuthorizationPending:
		case api.DeviceSlowDown:
			interval += 5 * time.Second
		case api.DeviceAccessDenied:
			return api.DeviceToken{}, authError{"the login was denied. No key was created"}
		case api.DeviceExpiredToken:
			return api.DeviceToken{}, authError{"the code expired. Run lillamolnet login again"}
		default:
			return api.DeviceToken{}, err
		}
		failures = 0
	}
	return api.DeviceToken{}, authError{"the code expired. Run lillamolnet login again"}
}

func newLogoutCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke the saved API key and remove it",
		Long: "Revoke the key that lillamolnet login saved, and remove it from this computer.\n" +
			"LILLAMOLNET_TOKEN is not touched.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			saved, err := loadCredentials(opts.getenv)
			if err != nil {
				return err
			}
			if saved == nil {
				_, err := fmt.Fprintln(opts.stdout, "Not logged in.")
				return err
			}

			client, err := api.New(saved.APIURL, saved.APIKey, currentVersion().Version)
			if err != nil {
				return err
			}

			revoked := true
			if err := client.RevokeCurrentKey(cmd.Context()); err != nil && !api.IsStatus(err, 401) {
				revoked = false
				fmt.Fprintf(opts.stderr, "The key could not be revoked (%s). Revoke %q at %s/users/settings/api_keys\n",
					err, saved.Name, strings.TrimRight(saved.APIURL, "/"))
			}

			if err := removeCredentials(opts.getenv); err != nil {
				return err
			}

			if strings.TrimSpace(opts.getenv(envToken)) != "" {
				fmt.Fprintf(opts.stderr, "Note: %s is still set.\n", envToken)
			}

			msg := "Logged out. The key is revoked and removed."
			if !revoked {
				msg = "Logged out. The key is removed from this computer."
			}
			_, err = fmt.Fprintln(opts.stdout, msg)
			return err
		},
	}
}

func defaultOpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
